package orderbookfiller

import (
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"go.uber.org/zap"

	clmath "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/math"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbookplugin"

	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

func (o *orderbookFillerIngestPlugin) getFillableOrders(ctx blockContext, canonicalOrderbookResult domain.CanonicalOrderBooksResult) (osmomath.Int, osmomath.Int, error) {
	ordersData, ok := o.orderMapByPoolID.Load(canonicalOrderbookResult.PoolID)
	if !ok {
		return osmomath.Int{}, osmomath.Int{}, fmt.Errorf("orderbook orders not found %d", canonicalOrderbookResult.PoolID)
	}

	orders, ok := ordersData.(orderbookplugindomain.OrdersResponse)
	if !ok {
		return osmomath.Int{}, osmomath.Int{}, fmt.Errorf("failed to cast order data %d", canonicalOrderbookResult.PoolID)
	}

	askOrders := orders.AskOrders
	bidOrders := orders.BidOrders

	orderBookPool, err := o.poolsUseCase.GetPool(canonicalOrderbookResult.PoolID)
	if err != nil {
		return osmomath.Int{}, osmomath.Int{}, err
	}
	orderBookState := orderBookPool.GetSQSPoolModel().CosmWasmPoolModel.Data.Orderbook

	pricesResult, err := o.tokensUseCase.GetPrices(ctx, []string{orderBookState.BaseDenom}, []string{orderBookState.QuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		o.logger.Error("failed to get prices", zap.Error(err))
		return osmomath.Int{}, osmomath.Int{}, err
	}

	price := pricesResult.GetPriceForDenom(orderBookState.BaseDenom, orderBookState.QuoteDenom)

	spotPriceScalingFactor, err := o.tokensUseCase.GetSpotPriceScalingFactorByDenom(orderBookState.QuoteDenom, orderBookState.BaseDenom)
	if err != nil {
		o.logger.Error("failed to get spot price scaling factor", zap.Error(err))
		return osmomath.Int{}, osmomath.Int{}, err
	}

	// Scale the price
	price.MulMut(osmomath.BigDecFromDec(spotPriceScalingFactor))

	// Create a map from ticks to cumulative total
	tickRemainingLiqMap := make(map[int64]cosmwasmpool.OrderbookTickLiquidity)
	for _, tick := range orderBookState.Ticks {
		tickRemainingLiqMap[tick.TickId] = tick.TickLiquidity
	}

	currentTick, err := clmath.CalculatePriceToTick(price)
	if err != nil {
		o.logger.Error("failed to calculate price to tick", zap.Error(err))
		return osmomath.Int{}, osmomath.Int{}, err
	}

	if len(askOrders) == 0 && len(bidOrders) == 0 {
		o.logger.Info("no orders found", zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
		return osmomath.Int{}, osmomath.Int{}, err
	}

	fillableAskAmountInQuoteDenom, err := o.getFillableAskAmountInQuoteDenom(askOrders, currentTick, tickRemainingLiqMap)
	if err != nil {
		fillableAskAmountInQuoteDenom = osmomath.ZeroInt()
		o.logger.Error("failed to get fillable ask amount in quote denom", zap.Error(err), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	fillableBidAmountInBaseDenom, err := o.getFillableBidAmountInBaseDenom(bidOrders, currentTick, tickRemainingLiqMap)
	if err != nil {
		fillableBidAmountInBaseDenom = osmomath.ZeroInt()
		o.logger.Error("failed to get fillable bid amount in base denom", zap.Error(err), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	return fillableAskAmountInQuoteDenom, fillableBidAmountInBaseDenom, nil
}

func (o *orderbookFillerIngestPlugin) getFillableAskAmountInQuoteDenom(askOrders []orderbookplugindomain.Order, currentTick int64, tickRemainingLiqMap map[int64]cosmwasmpool.OrderbookTickLiquidity) (osmomath.Int, error) {
	fillableAskAmountInQuoteDenom := osmomath.ZeroBigDec()

	// Multiple orders may be placed on the same tick, so we need to keep track of which ticks we have processed
	processedTickMap := make(map[int64]struct{})

	for _, order := range askOrders {
		if order.TickId < currentTick {
			_, hasProcessedTick := processedTickMap[order.TickId]
			if hasProcessedTick {
				continue
			}

			remainingTickLiq, ok := tickRemainingLiqMap[order.TickId]
			if !ok {
				return osmomath.Int{}, fmt.Errorf("ask liquidity not found for tick %d", order.TickId)
			}

			orderAmountAsk := remainingTickLiq.AskLiquidity

			tickPrice, err := clmath.TickToPrice(order.TickId)
			if err != nil {
				return osmomath.Int{}, err
			}

			curFillableAskAmountInQuoteDenom := cosmwasmpool.OrderbookValueInOppositeDirection(orderAmountAsk, tickPrice, cosmwasmpool.ASK, cosmwasmpool.ROUND_DOWN).TruncateDec()

			fillableAskAmountInQuoteDenom.AddMut(curFillableAskAmountInQuoteDenom)

			processedTickMap[order.TickId] = struct{}{}
		}
	}

	return fillableAskAmountInQuoteDenom.Dec().TruncateInt(), nil
}

func (o *orderbookFillerIngestPlugin) getFillableBidAmountInBaseDenom(bidOrders []orderbookplugindomain.Order, currentTick int64, tickRemainingLiqMap map[int64]cosmwasmpool.OrderbookTickLiquidity) (osmomath.Int, error) {
	fillableBidAmountInBaseDenom := osmomath.ZeroBigDec()

	// Multiple orders may be placed on the same tick, so we need to keep track of which ticks we have processed
	processedTickMap := make(map[int64]struct{})

	for _, order := range bidOrders {
		if order.TickId > currentTick {
			_, hasProcessedTick := processedTickMap[order.TickId]
			if hasProcessedTick {
				continue
			}

			remainingTickLiq, ok := tickRemainingLiqMap[order.TickId]
			if !ok {
				return osmomath.Int{}, fmt.Errorf("ask liquidity not found for tick %d", order.TickId)
			}

			orderAmountBid := remainingTickLiq.BidLiquidity

			tickPrice, err := clmath.TickToPrice(order.TickId)
			if err != nil {
				return osmomath.Int{}, err
			}

			curFillableAskAmountInQuoteDenom := cosmwasmpool.OrderbookValueInOppositeDirection(orderAmountBid, tickPrice, cosmwasmpool.BID, cosmwasmpool.ROUND_DOWN).TruncateDec()

			fillableBidAmountInBaseDenom.AddMut(curFillableAskAmountInQuoteDenom)

			processedTickMap[order.TickId] = struct{}{}
		}
	}

	return fillableBidAmountInBaseDenom.Dec().TruncateInt(), nil
}
