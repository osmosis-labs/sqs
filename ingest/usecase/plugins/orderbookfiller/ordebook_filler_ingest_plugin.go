package orderbookfiller

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbookplugin"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// orderbookFillerIngestPlugin is a plugin that fills the orderbook orders at the end of the block.
type orderbookFillerIngestPlugin struct {
	poolsUseCase  mvc.PoolsUsecase
	routerUseCase mvc.RouterUsecase
	tokensUseCase mvc.TokensUsecase

	passthroughGRPCClient passthroughdomain.PassthroughGRPCClient

	orderbookCWAAPIClient orderbookplugindomain.OrderbookCWAPIClient

	atomicBool        atomic.Bool
	keyring           keyring.Keyring
	defaultQuoteDenom string

	orderMapByPoolID sync.Map

	logger log.Logger
}

var _ domain.EndBlockProcessPlugin = &orderbookFillerIngestPlugin{}

const (
	// baseDenom is the base denom for the chain.
	baseDenom = "uosmo"
)

var (
	// minBalanceValueInUSDC is the minimum balance in USDC that has to be in the
	// orderbook pool to be considered for orderbook filling.
	minBalanceValueInUSDC = osmomath.NewInt(10)
)

func New(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, passthroughGRPCClient passthroughdomain.PassthroughGRPCClient, orderBookCWAPIClient orderbookplugindomain.OrderbookCWAPIClient, keyring keyring.Keyring, defaultQuoteDenom string, logger log.Logger) *orderbookFillerIngestPlugin {
	return &orderbookFillerIngestPlugin{
		poolsUseCase:  poolsUseCase,
		routerUseCase: routerUseCase,
		tokensUseCase: tokensUseCase,

		passthroughGRPCClient: passthroughGRPCClient,
		orderbookCWAAPIClient: orderBookCWAPIClient,

		atomicBool: atomic.Bool{},

		keyring: keyring,

		logger: logger,

		defaultQuoteDenom: defaultQuoteDenom,

		orderMapByPoolID: sync.Map{},
	}
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin.
func (o *orderbookFillerIngestPlugin) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
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

	if !o.atomicBool.CompareAndSwap(false, true) {
		o.logger.Info("orderbook filler is already in progress", zap.Uint64("block_height", blockHeight))
		return nil
	}
	defer o.atomicBool.Store(false)

	// Get unique denoms
	uniqueDenoms := o.getUniqueOrderbookDenoms(canonicalOrderbooks)

	// Configure block context
	blockCtx, err := o.newBlockContext(ctx, o.passthroughGRPCClient.GetChainGRPCClient(), uniqueDenoms)
	if err != nil {
		return err
	}

	type orderBookProcessResult struct {
		err    error
		poolID uint64
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

			err = o.processOrderbook(blockCtx, canonicalOrderbook)
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
	if err := o.tryFill(blockCtx); err != nil {
		o.logger.Error("failed to fill", zap.Error(err))
	}

	o.logger.Info("processed end block in orderbook filler ingest plugin", zap.Uint64("block_height", blockHeight))
	return nil
}

// getUniqueOrderbookDenoms returns the unique denoms from the canonical orderbooks.
func (*orderbookFillerIngestPlugin) getUniqueOrderbookDenoms(canonicalOrderbooks []domain.CanonicalOrderBooksResult) []string {
	// Map of denoms
	uniqueDenoms := make(map[string]struct{})
	for _, canonicalOrderbook := range canonicalOrderbooks {
		uniqueDenoms[canonicalOrderbook.Base] = struct{}{}
		uniqueDenoms[canonicalOrderbook.Quote] = struct{}{}
	}

	// Append base denom
	uniqueDenoms[baseDenom] = struct{}{}

	// Convert to unqiue slice
	denoms := make([]string, 0, len(uniqueDenoms))
	for denom := range uniqueDenoms {
		denoms = append(denoms, denom)
	}

	return denoms
}

func (o *orderbookFillerIngestPlugin) processOrderbook(ctx blockContext, canonicalOrderbookResult domain.CanonicalOrderBooksResult) error {
	baseDenom := canonicalOrderbookResult.Base
	quoteDenom := canonicalOrderbookResult.Quote

	baseAmountBalance := ctx.blockBalances.AmountOf(baseDenom)
	quoteAmountBalance := ctx.blockBalances.AmountOf(quoteDenom)

	isBaseLowBalance, err := o.shouldSkipLowBalance(baseDenom, baseAmountBalance, minBalanceValueInUSDC, ctx.prices)
	if err != nil {
		return err
	}

	if isBaseLowBalance {
		return nil
	}

	isQuoteLowBalance, err := o.shouldSkipLowBalance(quoteDenom, quoteAmountBalance, minBalanceValueInUSDC, ctx.prices)
	if err != nil {
		return err
	}

	if isQuoteLowBalance {
		return nil
	}

	fillableAskAmountQuoteDenom, fillableBidAmountBaseDenom, err := o.getFillableOrders(ctx, canonicalOrderbookResult)
	if err != nil {
		return err
	}

	if err := o.validateArb(ctx, fillableAskAmountQuoteDenom, canonicalOrderbookResult.Quote, canonicalOrderbookResult.Base, canonicalOrderbookResult.PoolID); err != nil {
		o.logger.Error("failed to fill asks", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID), zap.Error(err))
	} else {
		o.logger.Info("passed orderbook asks", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	if err := o.validateArb(ctx, fillableBidAmountBaseDenom, canonicalOrderbookResult.Base, canonicalOrderbookResult.Quote, canonicalOrderbookResult.PoolID); err != nil {
		o.logger.Error("failed to fill bids", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID), zap.Error(err))
	} else {
		o.logger.Info("passed orderbook bids", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	return nil
}

// validateArb validates the arb opportunity.
func (o *orderbookFillerIngestPlugin) validateArb(ctx blockContext, amountIn osmomath.Int, denomIn, denomOut string, orderBookID uint64) error {
	if amountIn.IsNil() || amountIn.IsZero() {
		return fmt.Errorf("estimated amount in truncated to zero")
	}

	coinIn := sdk.Coin{Denom: denomIn, Amount: amountIn}
	_, _, route, err := o.estimateArb(ctx, coinIn, denomOut, orderBookID)
	if err != nil {
		o.logger.Debug("failed to estimate arb", zap.Error(err))
		return err
	}

	// Simulate an individual swap
	msgContext, err := o.simulateSwapExactAmountIn(ctx, coinIn, route)
	if err != nil {
		return err
	}

	// If profitable, execute add the message to the transaction context
	ctx.txContext.AddMsg(msgContext)

	return nil
}

func (o *orderbookFillerIngestPlugin) tryFill(ctx blockContext) error {
	if len(ctx.txContext.msgs) == 0 {
		return nil
	}

	// Rank and filter pools
	ctx.txContext.rankAndFilterPools()

	// Simulate an individual swap
	_, adjustedGasAmount, err := o.simulateMsgs(ctx.getSDKMsgs())
	if err != nil {
		return err
	}

	// Updated gas amount
	ctx.txContext.UpdateAdjustedGasTotal(adjustedGasAmount)

	// Execute the swap
	_, _, err = o.executeTx(ctx)
	if err != nil {
		return err
	}

	return nil
}
