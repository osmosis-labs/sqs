package fillbot

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	cosmosbroadcast "github.com/osmosis-labs/osmoutil-go/tx/broadcast/cosmos"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// orderbookFillerIngestPlugin is a plugin that fills the orderbook orders at the end of the block.
type orderbookFillerIngestPlugin struct {
	poolsUseCase  mvc.PoolsUsecase
	routerUseCase mvc.RouterUsecase
	tokensUseCase mvc.TokensUsecase

	liquidityPricer domain.LiquidityPricer

	passthroughGRPCClient passthroughdomain.PassthroughGRPCClient

	orderbookCWAAPIClient orderbookplugindomain.OrderbookCWAPIClient

	atomicBool atomic.Bool

	orderMapByPoolID  sync.Map
	signer            cosmosbroadcast.CosmosSigner
	defaultQuoteDenom string

	logger log.Logger
}

var _ domain.EndBlockProcessPlugin = &orderbookFillerIngestPlugin{}

type orderBookProcessResult struct {
	err    error
	poolID uint64
}

const (
	tracerName = "sqs-orderbook-filler"
)

var (
	tracer = otel.Tracer(tracerName)
)

func New(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, passthroughGRPCClient passthroughdomain.PassthroughGRPCClient, orderBookCWAPIClient orderbookplugindomain.OrderbookCWAPIClient, signer cosmosbroadcast.CosmosSigner, defaultQuoteDenom string, logger log.Logger) *orderbookFillerIngestPlugin {
	liquidityPricer := worker.NewLiquidityPricer(defaultQuoteDenom, tokensUseCase.GetChainScalingFactorByDenomMut)

	return &orderbookFillerIngestPlugin{
		poolsUseCase:  poolsUseCase,
		routerUseCase: routerUseCase,
		tokensUseCase: tokensUseCase,

		passthroughGRPCClient: passthroughGRPCClient,
		orderbookCWAAPIClient: orderBookCWAPIClient,

		atomicBool: atomic.Bool{},

		orderMapByPoolID: sync.Map{},

		defaultQuoteDenom: defaultQuoteDenom,

		liquidityPricer: liquidityPricer,

		signer: signer,

		logger: logger,
	}
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin.
func (o *orderbookFillerIngestPlugin) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	ctx, span := tracer.Start(ctx, "orderbookFillerIngestPlugin.ProcessEndBlock")
	defer span.End()

	canonicalOrderbooks, err := o.poolsUseCase.GetAllCanonicalOrderbookPoolIDs()
	if err != nil {
		o.logger.Error("failed to get all canonical orderbook pool IDs", zap.Error(err))
		return err
	}

	// TODO: encapsulate and parallelize this
	// Fetch ticks for all the orderbooks
	for _, canonicalOrderbookResult := range canonicalOrderbooks {
		if _, ok := metadata.PoolIDs[canonicalOrderbookResult.PoolID]; ok {
			if err := o.fetchTicksForOrderbook(ctx, canonicalOrderbookResult); err != nil {
				o.logger.Error("failed to fetch ticks for orderbook", zap.Error(err), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
				return err
			}
		}
	}

	// For simplicity, we allow only one block to be processed at a time.
	// This may be relaxed in the future.
	if !o.atomicBool.CompareAndSwap(false, true) {
		o.logger.Info("orderbook filler is already in progress", zap.Uint64("block_height", blockHeight))
		return nil
	}
	defer o.atomicBool.Store(false)

	// Get unique orderbook denoms
	uniqueOrderBookDenoms := o.getUniqueOrderbookDenoms(canonicalOrderbooks)

	// Get bot balances
	balances, err := o.passthroughGRPCClient.AllBalances(ctx, o.signer.Address().String())
	if err != nil {
		return err
	}

	// Get prices for all the unique denoms in the orderbook, including base denom.
	orderBookDenomPrices, err := o.tokensUseCase.GetPrices(ctx, uniqueOrderBookDenoms, []string{o.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return err
	}

	// Configure block context
	blockCtx := newBlockContext(ctx, orderBookDenomPrices, balances, o.defaultQuoteDenom, blockHeight)

	resultChan := make(chan orderBookProcessResult, len(canonicalOrderbooks))
	defer close(resultChan)

	for _, canonicalOrderbook := range canonicalOrderbooks {
		go func(canonicalOrderbook domain.CanonicalOrderBooksResult) {
			var err error

			defer func() {
				resultChan <- orderBookProcessResult{
					err:    err,
					poolID: canonicalOrderbook.PoolID,
				}
			}()

			err = o.processOrderBook(blockCtx, canonicalOrderbook)
		}(canonicalOrderbook)
	}

	// Collect all the results
	for range canonicalOrderbooks {
		select {
		case result := <-resultChan:
			if result.err != nil {
				o.logger.Error("failed to process orderbook", zap.Error(result.err))
			}
		case <-blockCtx.ctx.Done():
			o.logger.Debug("context cancelled processing orderbook")
		case <-time.After(100 * time.Second):
			o.logger.Debug("timeout processing orderbook")
		}
	}

	if err := o.fillWithFallback(blockCtx, o.tryFill); err != nil {
		return err
	}

	o.logger.Info("processed end block in orderbook filler ingest plugin", zap.Uint64("block_height", blockHeight))
	return nil
}

// fillWithFallback attempts to execute all collected messages as a single batch tx.
// If the batch fails and more than one non-overlapping message survived rankAndFilterMsgs,
// it falls back to executing only those surviving messages individually so dropped
// overlapping messages are never executed with stale profitability estimates.
func (o *orderbookFillerIngestPlugin) fillWithFallback(blockCtx *blockContext, tryFill func(*blockContext, *txContext) error) error {
	err := tryFill(blockCtx, blockCtx.tx)
	if err == nil {
		return nil
	}

	filteredMsgs := blockCtx.tx.msgs
	if len(filteredMsgs) <= 1 {
		o.logger.Error("failed to fill", zap.Error(err))
		return err
	}

	o.logger.Error("failed to fill batch of arbs as one tx - falling back to executing each message as separate tx", zap.Error(err))
	for _, msg := range filteredMsgs {
		curTx := newTxContext()
		curTx.addMsg(msg)

		if err := tryFill(blockCtx, curTx); err != nil {
			o.logger.Error("failed to fill individual msg tx", zap.Error(err))
		}
	}

	return nil
}

// getUniqueOrderbookDenoms returns the unique denoms from the canonical orderbooks and the chain base denom.s
func (*orderbookFillerIngestPlugin) getUniqueOrderbookDenoms(canonicalOrderbooks []domain.CanonicalOrderBooksResult) []string {
	// Map of denoms
	uniqueDenoms := make(map[string]struct{})
	for _, canonicalOrderbook := range canonicalOrderbooks {
		uniqueDenoms[canonicalOrderbook.Base] = struct{}{}
		uniqueDenoms[canonicalOrderbook.Quote] = struct{}{}
	}

	// Append base denom
	uniqueDenoms[orderbookplugindomain.BaseDenom] = struct{}{}

	// Convert to unqiue slice
	denoms := make([]string, 0, len(uniqueDenoms))
	for denom := range uniqueDenoms {
		denoms = append(denoms, denom)
	}

	return denoms
}

// processOrderBook processes the orderbook in the following sequence:
// - Validate user balances meeting minimum threshold.
// - Compute fillable amounts for the order book.
// - Validate arb trying to fill ask liquidity.
// - Validate arb trying to fill bid liquidity.
// - Returns error if any of the steps fail.
//
// If validation/simulation passes, the message is added to the transaction context to be execute in batch at the end of the block.
func (o *orderbookFillerIngestPlugin) processOrderBook(blockCtx *blockContext, canonicalOrderbookResult domain.CanonicalOrderBooksResult) error {
	baseDenom := canonicalOrderbookResult.Base
	quoteDenom := canonicalOrderbookResult.Quote
	_, span := tracer.Start(blockCtx.ctx, "orderbookFillerIngestPlugin.processOrderBook")
	defer span.End()

	span.SetAttributes(attribute.Int64("orderbook_id", int64(canonicalOrderbookResult.PoolID)))

	// Validate user balances meeting minimum threshold.
	if err := o.validateUserBalances(blockCtx, baseDenom, quoteDenom); err != nil {
		return err
	}

	// Compute fillable amounts for the order book.
	fillableAskAmountQuoteDenom, fillableBidAmountBaseDenom, err := o.getFillableOrders(blockCtx, canonicalOrderbookResult)
	if err != nil {
		return err
	}

	// Choose max value between fillable amount and user balance
	// This is so that we can at least partially fill if the user balance is less than the fillable amount.
	userBalanceQuoteDenom := blockCtx.userBalances.AmountOf(quoteDenom)
	if userBalanceQuoteDenom.LT(fillableAskAmountQuoteDenom) {
		fillableAskAmountQuoteDenom = userBalanceQuoteDenom
		o.logger.Warn("user balance less than fillable ask amount", zap.String("quote_denom", quoteDenom), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	userBalanceBaseDenom := blockCtx.userBalances.AmountOf(baseDenom)
	if userBalanceBaseDenom.LT(fillableBidAmountBaseDenom) {
		fillableBidAmountBaseDenom = userBalanceBaseDenom
		o.logger.Warn("user balance less than fillable bid amount", zap.String("base_denom", baseDenom), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	// Validate arb trying to fill ask liquidity.
	if _, err := o.computePerfectArbAmountIfExists(blockCtx, fillableAskAmountQuoteDenom, canonicalOrderbookResult.Quote, canonicalOrderbookResult.Base, canonicalOrderbookResult.PoolID); err != nil {
		o.logger.Error("failed to fill asks", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID), zap.Error(err))
	} else {
		o.logger.Info("passed orderbook asks", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	// Validate arb trying to fill bid liquidity
	if _, err := o.computePerfectArbAmountIfExists(blockCtx, fillableBidAmountBaseDenom, canonicalOrderbookResult.Base, canonicalOrderbookResult.Quote, canonicalOrderbookResult.PoolID); err != nil {
		o.logger.Error("failed to fill bids", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID), zap.Error(err))
	} else {
		o.logger.Info("passed orderbook bids", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	return nil
}

const (
	maxRecursionAttemptsArbSearch = 15
)

var (
	multiplier = osmomath.MustNewDecFromStr("1.05")
	two        = osmomath.MustNewDecFromStr("2")
)

// computePerfectArbAmountIfExists computes the perfect arb amount if it exists by performing binary search.
// It tries to prefer a higher amount if it exists in order to fill all orders in-full while maximizing profit.
// nolint: unparam
func (o *orderbookFillerIngestPlugin) computePerfectArbAmountIfExists(blockCtx *blockContext, proposedAmountIn osmomath.Int, denomIn, denomOut string, orderBookID uint64) (osmomath.Int, error) {
	// If the initial proposed amount in is not valid, return error.
	msgCtx, err := o.validateArb(blockCtx, proposedAmountIn, denomIn, denomOut, orderBookID)
	if err != nil {
		return osmomath.Int{}, err
	}

	// Otherwise, try to find a higher amount such that it fills all orders in-full and is profitable.
	amountInHigh := proposedAmountIn.ToLegacyDec().MulMut(multiplier).TruncateInt()

	msgCtx, result, err := o.tryValidate(blockCtx, proposedAmountIn, amountInHigh, denomIn, denomOut, orderBookID, msgCtx, maxRecursionAttemptsArbSearch)
	if err != nil {
		return proposedAmountIn, nil
	}

	// If profitable, add the message to the transaction context
	blockCtx.tx.addMsg(msgCtx)

	return result, nil
}

func (o *orderbookFillerIngestPlugin) tryValidate(blockCtx *blockContext, amountInLow osmomath.Int, amountInHigh osmomath.Int, denomIn, denomOut string, orderBookID uint64, lowMsgCtx msgContext, attemptsRemaining int) (msgContext, osmomath.Int, error) {
	if attemptsRemaining == 0 {
		return lowMsgCtx, amountInLow, nil
	}

	mid := amountInLow.ToLegacyDec().Add(amountInHigh.ToLegacyDec()).QuoRoundupMut(two).Ceil().TruncateInt()

	// Case 1: mid arb works => recurse into (mid, high)
	midMsgCtx, err := o.validateArb(blockCtx, mid, denomIn, denomOut, orderBookID)
	if err == nil && midMsgCtx.maxFeeCap.GTE(lowMsgCtx.maxFeeCap) {
		return o.tryValidate(blockCtx, mid, amountInHigh, denomIn, denomOut, orderBookID, midMsgCtx, attemptsRemaining-1)
	}

	// Case 2: mid arb doesn't work => recurse into (low, mid)
	topMsgCtx, topAmount, err := o.tryValidate(blockCtx, amountInLow, mid, denomIn, denomOut, orderBookID, lowMsgCtx, attemptsRemaining-1)
	if err == nil && topMsgCtx.maxFeeCap.GTE(lowMsgCtx.maxFeeCap) {
		return topMsgCtx, topAmount, nil
	}

	// Case 3: all attempts failed but low arb has been validated in the caller => return it.
	return lowMsgCtx, amountInLow, nil
}

// tryFill tries to fill the orderbook by executing the transaction.
// It ranks and filters the pools, simulates the transaction messages, and executes the swap if the simulation passes.
func (o *orderbookFillerIngestPlugin) tryFill(blockCtx *blockContext, txCtx *txContext) error {
	if len(txCtx.msgs) == 0 {
		return nil
	}

	// Rank and filter pools
	txCtx.rankAndFilterMsgs()
	if len(txCtx.msgs) == 0 {
		return nil
	}

	// Simulate transaction messages
	_, adjustedGasAmount, err := o.simulateMsgs(blockCtx.ctx, txCtx.sdkMsgs())
	if err != nil {
		return err
	}

	// Update adjusted gas amount upon resimulating the transaction.
	txCtx.adjustedGasUsedTotal = adjustedGasAmount

	// Execute the swap
	_, _, err = o.executeTx(blockCtx, txCtx)
	return err
}
