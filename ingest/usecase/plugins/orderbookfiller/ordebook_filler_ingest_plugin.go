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
				o.logger.Error("failed to process orderbook", zap.Error(err))
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

	baseDenomPrice := prices.GetPriceForDenom(baseDenom, o.defaultQuoteDenom)

	// Calculate amount equivalent to $10 in USDC for baseDenom and quoteDenom
	baseAmountInUSDC := osmomath.NewBigDec(10_000_000).Quo(baseDenomPrice) // Assuming prices are in USDC terms

	// Base scaling factor
	scalingFactor, err := o.tokensUseCase.GetChainScalingFactorByDenomMut(baseDenom)
	if err != nil {
		return err
	}

	// Scale the base amount
	baseAmountInUSDC = baseAmountInUSDC.Mul(osmomath.BigDecFromDec(scalingFactor))

	// Make it $10 in USDC terms for baseDenom
	baseInCoin := sdk.NewCoin(baseDenom, baseAmountInUSDC.Dec().TruncateInt())
	baseInOrderbookQuote, err := o.routerUseCase.GetCustomDirectQuote(ctx, baseInCoin, quoteDenom, canonicalOrderbookResult.PoolID)
	if err != nil {
		return err
	}

	// Make it $10 in USDC terms for quoteDenom
	quoteInCoin := sdk.NewCoin(quoteDenom, baseInOrderbookQuote.GetAmountOut())
	cyclicArbQuote, err := o.routerUseCase.GetSimpleQuote(ctx, quoteInCoin, baseDenom, domain.WithDisableSplitRoutes())
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

			// Construct the rotue and push it to message signing logic.

			// Additional logic to verify swap, update state, etc.
		}
	}

	return nil
}
