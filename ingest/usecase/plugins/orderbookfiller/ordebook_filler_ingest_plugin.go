package orderbookfiller

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbookplugin"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	blockctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbookfiller/context/block"
	txctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbookfiller/context/tx"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
	"go.opentelemetry.io/otel"
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
	keyring           keyring.Keyring
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

func New(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, passthroughGRPCClient passthroughdomain.PassthroughGRPCClient, orderBookCWAPIClient orderbookplugindomain.OrderbookCWAPIClient, keyring keyring.Keyring, defaultQuoteDenom string, logger log.Logger) *orderbookFillerIngestPlugin {
	liquidityPricer := worker.NewLiquidityPricer(defaultQuoteDenom, tokensUseCase.GetChainScalingFactorByDenomMut)

	return &orderbookFillerIngestPlugin{
		poolsUseCase:  poolsUseCase,
		routerUseCase: routerUseCase,
		tokensUseCase: tokensUseCase,

		passthroughGRPCClient: passthroughGRPCClient,
		orderbookCWAAPIClient: orderBookCWAPIClient,

		atomicBool: atomic.Bool{},

		orderMapByPoolID: sync.Map{},

		keyring:           keyring,
		defaultQuoteDenom: defaultQuoteDenom,

		liquidityPricer: liquidityPricer,

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
	balances, err := o.passthroughGRPCClient.AllBalances(ctx, o.keyring.GetAddress().String())
	if err != nil {
		return err
	}

	// Get prices for all the unique denoms in the orderbook, including base denom.
	orderBookDenomPrices, err := o.tokensUseCase.GetPrices(ctx, uniqueOrderBookDenoms, []string{o.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return err
	}

	// Configure block context
	blockCtx, err := blockctx.New(ctx, o.passthroughGRPCClient.GetChainGRPCClient(), uniqueOrderBookDenoms, orderBookDenomPrices, balances, o.defaultQuoteDenom)
	if err != nil {
		return err
	}

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
	for i := 0; i < len(canonicalOrderbooks); i++ {
		select {
		case result := <-resultChan:
			if result.err != nil {
				o.logger.Error("failed to process orderbook", zap.Error(result.err))
			}
		case <-blockCtx.Done():
			o.logger.Debug("context cancelled processing orderbook")
		case <-time.After(100 * time.Second):
			o.logger.Debug("timeout processing orderbook")
		}
	}

	// Execute tx
	txCtx := blockCtx.GetTxCtx()
	blockGasPrice := blockCtx.GetGasPrice()
	if err := o.tryFill(txCtx, blockGasPrice); err != nil {
		o.logger.Error("failed to fill", zap.Error(err))
	}

	o.logger.Info("processed end block in orderbook filler ingest plugin", zap.Uint64("block_height", blockHeight))
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
func (o *orderbookFillerIngestPlugin) processOrderBook(ctx blockctx.BlockCtxI, canonicalOrderbookResult domain.CanonicalOrderBooksResult) error {
	baseDenom := canonicalOrderbookResult.Base
	quoteDenom := canonicalOrderbookResult.Quote

	// Validate user balances meeting minimum threshold.
	if err := o.validateUserBalances(ctx, baseDenom, quoteDenom); err != nil {
		return err
	}

	// Compute fillable amounts for the order book.
	fillableAskAmountQuoteDenom, fillableBidAmountBaseDenom, err := o.getFillableOrders(ctx, canonicalOrderbookResult)
	if err != nil {
		return err
	}

	// Validate arb trying to fill ask liquidity.
	if err := o.validateArb(ctx, fillableAskAmountQuoteDenom, canonicalOrderbookResult.Quote, canonicalOrderbookResult.Base, canonicalOrderbookResult.PoolID); err != nil {
		o.logger.Error("failed to fill asks", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID), zap.Error(err))
	} else {
		o.logger.Info("passed orderbook asks", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	// Validae arb trying to fill bid liquidity
	if err := o.validateArb(ctx, fillableBidAmountBaseDenom, canonicalOrderbookResult.Base, canonicalOrderbookResult.Quote, canonicalOrderbookResult.PoolID); err != nil {
		o.logger.Error("failed to fill bids", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID), zap.Error(err))
	} else {
		o.logger.Info("passed orderbook bids", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	return nil
}

// tryFill tries to fill the orderbook by executing the transaction.
// It ranks and filters the pools, simulates the transaction messages, and executes the swap if the simulation passes.
func (o *orderbookFillerIngestPlugin) tryFill(txCtx txctx.TxContextI, blockGasPrice blockctx.BlockGasPrice) error {
	msgs := txCtx.GetSDKMsgs()

	if len(msgs) == 0 {
		return nil
	}

	// Rank and filter pools
	txCtx.RankAndFilterMsgs()

	// Simulate transaction messages
	sdkMsgs := txCtx.GetSDKMsgs()
	_, adjustedGasAmount, err := o.simulateMsgs(sdkMsgs)
	if err != nil {
		return err
	}

	// Update adjusted gas amount upon resimulating the transaction.
	txCtx.UpdateAdjustedGasTotal(adjustedGasAmount)

	// Execute the swap
	_, _, err = o.executeTx(txCtx, blockGasPrice)
	if err != nil {
		return err
	}

	return nil
}
