package orderbookfiller

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	blockctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbookfiller/context/block"
	"go.uber.org/zap"
)

func (o *orderbookFillerIngestPlugin) estimateArb(ctx blockctx.BlockCtxI, coinIn sdk.Coin, denomOut string, canonicalOrderbookPoolId uint64) (osmomath.Int, osmomath.Int, []domain.RoutablePool, error) {
	o.logger.Debug("estimating cyclic arb", zap.Uint64("orderbook_id", canonicalOrderbookPoolId), zap.Stringer("denom_in", coinIn), zap.String("denom_out", denomOut))

	goCtx := ctx.AsGoCtx()

	baseInOrderbookQuote, err := o.routerUseCase.GetCustomDirectQuote(goCtx, coinIn, denomOut, canonicalOrderbookPoolId)
	if err != nil {
		return osmomath.Int{}, osmomath.Int{}, nil, err
	}

	// Make it $10 in USDC terms for quoteDenom
	quoteInCoin := sdk.NewCoin(denomOut, baseInOrderbookQuote.GetAmountOut())
	cyclicArbQuote, err := o.routerUseCase.GetSimpleQuote(goCtx, quoteInCoin, coinIn.Denom, domain.WithDisableSplitRoutes())
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
