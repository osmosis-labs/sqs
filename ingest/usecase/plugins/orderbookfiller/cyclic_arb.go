package orderbookfiller

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	blockctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbookfiller/context/block"
	"go.uber.org/zap"
)

// estimateCyclicArb estimates the cyclic arb by swapping coinIn against the orderbook and then finding an optimal route from the denomOut
// back to the denom in. Constructs a final cyclic route from the denom in to out and back. Also returns, the final inverse amount out in the denomination
// of the coin in denom.
func (o *orderbookFillerIngestPlugin) estimateCyclicArb(ctx blockctx.BlockCtxI, coinIn sdk.Coin, denomOut string, canonicalOrderbookPoolId uint64) (osmomath.Int, []domain.RoutablePool, error) {
	o.logger.Debug("estimating cyclic arb", zap.Uint64("orderbook_id", canonicalOrderbookPoolId), zap.Stringer("denom_in", coinIn), zap.String("denom_out", denomOut))

	goCtx := ctx.AsGoCtx()

	baseInOrderbookQuote, err := o.routerUseCase.GetCustomDirectQuote(goCtx, coinIn, denomOut, canonicalOrderbookPoolId)
	if err != nil {
		return osmomath.Int{}, nil, err
	}

	// Make it $10 in USDC terms for quoteDenom
	quoteInCoin := sdk.NewCoin(denomOut, baseInOrderbookQuote.GetAmountOut())
	cyclicArbQuote, err := o.routerUseCase.GetSimpleQuote(goCtx, quoteInCoin, coinIn.Denom, domain.WithDisableSplitRoutes())
	if err != nil {
		return osmomath.Int{}, nil, err
	}

	inverseAmountIn := cyclicArbQuote.GetAmountOut()

	routeThere := baseInOrderbookQuote.GetRoute()
	if len(routeThere) != 1 {
		return osmomath.Int{}, nil, fmt.Errorf("route there should have 1 route")
	}

	routeBack := cyclicArbQuote.GetRoute()
	if len(routeBack) != 1 {
		return osmomath.Int{}, nil, fmt.Errorf("route back should have 1 route")
	}

	fullCyclicArbRoute := append(routeThere[0].GetPools(), routeBack[0].GetPools()...)

	return inverseAmountIn, fullCyclicArbRoute, nil
}

// validateArb validates the arb opportunity by constructing a route from SQS router and then simulating it against chain. 
func (o *orderbookFillerIngestPlugin) validateArb(ctx blockctx.BlockCtxI, amountIn osmomath.Int, denomIn, denomOut string, orderBookID uint64) error {
	if amountIn.IsNil() || amountIn.IsZero() {
		return fmt.Errorf("estimated amount in truncated to zero")
	}

	coinIn := sdk.Coin{Denom: denomIn, Amount: amountIn}
	_, route, err := o.estimateCyclicArb(ctx, coinIn, denomOut, orderBookID)
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
	txCtx := ctx.GetTxCtx()
	txCtx.AddMsg(msgContext)

	return nil
}
