package orderbookfiller

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"go.uber.org/zap"
)

func (o *orderbookFillerIngestPlugin) estimateArb(ctx blockContext, coinIn sdk.Coin, denomOut string, canonicalOrderbookPoolId uint64) (osmomath.Int, osmomath.Int, []domain.RoutablePool, error) {
	o.logger.Debug("estimating cyclic arb", zap.Uint64("orderbook_id", canonicalOrderbookPoolId), zap.Stringer("denom_in", coinIn), zap.String("denom_out", denomOut))

	baseInOrderbookQuote, err := o.routerUseCase.GetCustomDirectQuote(ctx, coinIn, denomOut, canonicalOrderbookPoolId)
	if err != nil {
		return osmomath.Int{}, osmomath.Int{}, nil, err
	}

	// Make it $10 in USDC terms for quoteDenom
	quoteInCoin := sdk.NewCoin(denomOut, baseInOrderbookQuote.GetAmountOut())
	cyclicArbQuote, err := o.routerUseCase.GetSimpleQuote(ctx, quoteInCoin, coinIn.Denom, domain.WithDisableSplitRoutes())
	if err != nil {
		return osmomath.Int{}, osmomath.Int{}, nil, err
	}

	inverseAmountIn := cyclicArbQuote.GetAmountOut()

	routeThere := baseInOrderbookQuote.GetRoute()
	if len(routeThere) != 1 {
		return osmomath.Int{}, osmomath.Int{}, nil, fmt.Errorf("route there should have 1 route")
	}

	routeBack := cyclicArbQuote.GetRoute()
	if len(routeBack) != 1 {
		return osmomath.Int{}, osmomath.Int{}, nil, fmt.Errorf("route back should have 1 route")
	}

	fullCyclicArbRoute := append(routeThere[0].GetPools(), routeBack[0].GetPools()...)

	return coinIn.Amount, inverseAmountIn, fullCyclicArbRoute, nil
}

// nolint: unused
func (o *orderbookFillerIngestPlugin) detectArb(ctx blockContext, amountInUSDCValue osmomath.BigDec, denomIn, denomOut string, canonicalOrderbookPoolId uint64) error {
	amountIn, err := o.usdcToDenomVlaue(denomIn, amountInUSDCValue.Dec(), ctx.prices)
	if err != nil {
		return err
	}

	coinIn := sdk.Coin{Denom: denomIn, Amount: amountIn}

	// Estimate the arb
	originalAmountIn, amountOut, cyclicArbRoute, err := o.estimateArb(ctx, coinIn, denomOut, canonicalOrderbookPoolId)
	if err != nil {
		return err
	}

	if originalAmountIn.LT(amountOut) {
		// Profitable arbitrage opportunity exists

		// Check if atomic operation is already in progress
		if o.atomicBool.CompareAndSwap(false, true) {
			defer o.atomicBool.Store(false)

			// Execute the swap
			// TODO:
			coinIn := sdk.Coin{Denom: denomIn, Amount: originalAmountIn}

			fmt.Printf("Profitable arbitrage opportunity exists: %s \n", amountInUSDCValue)

			fmt.Println("amountInUSDCValue", amountInUSDCValue)
			fmt.Println("coinIn", coinIn)
			fmt.Printf("cyclicArbRoute: %v \n", cyclicArbRoute)

			// Simulate arb
			msgCtx, err := o.simulateSwapExactAmountIn(ctx, coinIn, cyclicArbRoute)
			if err != nil {
				return err
			}

			ctx.txContext.AddMsg(msgCtx)

			_, _, err = o.executeTx(ctx)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
