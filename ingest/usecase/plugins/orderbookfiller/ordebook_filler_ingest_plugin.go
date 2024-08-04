package orderbookfiller

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// orderbookFillerIngestPlugin is a plugin that fills the orderbook orders at the end of the block.
type orderbookFillerIngestPlugin struct {
	poolsUseCase  mvc.PoolsUsecase
	routerUseCase mvc.RouterUsecase
	tokensUseCase mvc.TokensUsecase

	atomicBool atomic.Bool

	keyring           keyring.Keyring
	defaultQuoteDenom string

	logger log.Logger

	swapDone atomic.Bool
}

var _ domain.EndBlockProcessPlugin = &orderbookFillerIngestPlugin{}

func New(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, keyring keyring.Keyring, defaultQuoteDenom string, logger log.Logger) *orderbookFillerIngestPlugin {
	return &orderbookFillerIngestPlugin{
		poolsUseCase:  poolsUseCase,
		routerUseCase: routerUseCase,
		tokensUseCase: tokensUseCase,

		atomicBool: atomic.Bool{},

		keyring:           keyring,
		defaultQuoteDenom: defaultQuoteDenom,

		logger: logger,
	}
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin.
func (o *orderbookFillerIngestPlugin) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	canonicalOrderbooks, err := o.poolsUseCase.GetAllCanonicalOrderbookPoolIDs()
	if err != nil {
		o.logger.Error("failed to get all canonical orderbook pool IDs", zap.Error(err))
		return err
	}

	resultChan := make(chan error, len(canonicalOrderbooks))
	defer close(resultChan)

	for _, canonicalOrderbook := range canonicalOrderbooks {
		go func(canonicalOrderbook domain.CanonicalOrderBooksResult) {
			resultChan <- o.processOrderbook(ctx, canonicalOrderbook)
		}(canonicalOrderbook)
	}

	// Collect all the results
	for i := 0; i < len(canonicalOrderbooks); i++ {
		select {
		case err := <-resultChan:
			if err != nil {
				o.logger.Debug("failed to process orderbook", zap.Error(err))
			}
		case <-time.After(10 * time.Second):
			o.logger.Error("timed out processing orderbook")
			return err
		}
	}

	o.logger.Info("processed end block in orderbook filler ingest plugin", zap.Uint64("block_height", blockHeight))
	return nil
}

func (o *orderbookFillerIngestPlugin) processOrderbook(ctx context.Context, canonicalOrderbookResult domain.CanonicalOrderBooksResult) error {
	baseDenom := canonicalOrderbookResult.Base
	quoteDenom := canonicalOrderbookResult.Quote

	prices, err := o.tokensUseCase.GetPrices(ctx, []string{baseDenom, quoteDenom}, []string{o.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return err
	}

	// Detect arb swapping from base to quote (to pick up orders in one direction)
	err = o.detectArb(ctx, prices, baseDenom, quoteDenom, canonicalOrderbookResult.PoolID)
	if err != nil {
		o.logger.Debug("failed to detect arb", zap.Error(err), zap.String("denom_in", baseDenom), zap.String("denom_out", quoteDenom), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	// Detect arb swapping from quote to base (to pick up orders in the other direction)
	err = o.detectArb(ctx, prices, quoteDenom, baseDenom, canonicalOrderbookResult.PoolID)
	if err != nil {
		o.logger.Debug("failed to detect arb", zap.Error(err), zap.String("denom_in", quoteDenom), zap.String("denom_out", baseDenom), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	return nil
}

func (o *orderbookFillerIngestPlugin) detectArb(ctx context.Context, prices domain.PricesResult, denomIn, denomOut string, canonicalOrderbookPoolId uint64) error {
	denomInPrice := prices.GetPriceForDenom(denomIn, o.defaultQuoteDenom)

	// Calculate amount equivalent to $10 in USDC for baseDenom and quoteDenom
	amountInUSDC := osmomath.NewBigDec(10).Quo(denomInPrice) // Assuming prices are in USDC terms

	// Base scaling factor
	scalingFactor, err := o.tokensUseCase.GetChainScalingFactorByDenomMut(denomIn)
	if err != nil {
		return err
	}

	// Scale the base amount
	amountInUSDC = amountInUSDC.Mul(osmomath.BigDecFromDec(scalingFactor))

	// Make it $10 in USDC terms for baseDenom
	baseInCoin := sdk.NewCoin(denomIn, amountInUSDC.Dec().TruncateInt())

	o.logger.Info("estimating cyclic arb", zap.Uint64("orderbook_id", canonicalOrderbookPoolId), zap.Stringer("denom_in", baseInCoin), zap.String("denom_out", denomOut))

	baseInOrderbookQuote, err := o.routerUseCase.GetCustomDirectQuote(ctx, baseInCoin, denomOut, canonicalOrderbookPoolId)
	if err != nil {
		return err
	}

	// Make it $10 in USDC terms for quoteDenom
	quoteInCoin := sdk.NewCoin(denomOut, baseInOrderbookQuote.GetAmountOut())
	cyclicArbQuote, err := o.routerUseCase.GetSimpleQuote(ctx, quoteInCoin, denomIn, domain.WithDisableSplitRoutes())
	if err != nil {
		return err
	}

	errTolerance := osmomath.ErrTolerance{
		MultiplicativeTolerance: sdk.NewDecWithPrec(5, 2),
	}

	//
	difference := errTolerance.Compare(baseInCoin.Amount, cyclicArbQuote.GetAmountOut())

	if difference < 0 {
		// Profitable arbitrage opportunity exists

		// Check if atomic operation is already in progress
		if o.atomicBool.CompareAndSwap(false, true) {
			defer o.atomicBool.Store(false)

			// Execute the swap
			// TODO:
			fmt.Println("Profitable arbitrage opportunity exists")

			routeThere := baseInOrderbookQuote.GetRoute()
			if len(routeThere) != 1 {
				return fmt.Errorf("route there should have 1 route")
			}

			routeBack := cyclicArbQuote.GetRoute()
			if len(routeBack) != 1 {
				return fmt.Errorf("route back should have 1 route")
			}

			fullCyclicArbRoute := append(routeThere[0].GetPools(), routeBack[0].GetPools()...)

			_, _, err := o.swapExactAmountIn(baseInCoin, fullCyclicArbRoute)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
