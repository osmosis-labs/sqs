package orderbookfiller

import (
	"context"
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
	quoteDenomPrice := prices.GetPriceForDenom(quoteDenom, o.defaultQuoteDenom)

	// TODO: make it $10 in USDC terms by using prices above
	baseInCoin := sdk.NewCoin(baseDenom, sdk.NewInt(10_000_000))
	baseInOrderbookQuote, err := o.routerUseCase.GetCustomDirectQuote(ctx, baseInCoin, quoteDenom, canonicalOrderbookResult.PoolID)
	if err != nil {
		return err
	}

	// Get execution price
	baseOrderbookAmount := baseInOrderbookQuote.GetAmountIn()
	quoteOrderbookAmount := baseInOrderbookQuote.GetAmountOut()

	orderbookQuoteExecutionPrice := baseOrderbookAmount.Amount.ToLegacyDec().Quo(quoteOrderbookAmount.ToLegacyDec())

	// TODO: make it $10 in USDC terms by using prices above
	quoteInCoin := sdk.NewCoin(quoteDenom, sdk.NewInt(10_000_000))

	quoteInOptimalQuote, err := o.routerUseCase.GetSimpleQuote(ctx, quoteInCoin, baseDenom)
	if err != nil {
		return err
	}
	baseOptimalQuote := quoteInOptimalQuote.GetAmountOut()
	quoteOptimalQuote := quoteInOptimalQuote.GetAmountIn()

	// Get execution price
	optimalQuoteExecutionPrice := baseOptimalQuote.ToLegacyDec().Quo(quoteOptimalQuote.Amount.ToLegacyDec())

	errTolerance := osmomath.ErrTolerance{
		MultiplicativeTolerance: sdk.NewDecWithPrec(5, 2),
	}

	difference := errTolerance.CompareDec(orderbookQuoteExecutionPrice, optimalQuoteExecutionPrice)

	// TODO:
	// do one swap using keyring
	// Have an atomic.Bool, check if we the swap was done
	// Swap 2000uosmo to validate that everything works end-to-end
	return nil
}
