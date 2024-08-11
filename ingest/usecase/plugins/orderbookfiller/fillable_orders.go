package orderbookfiller

import (
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	blockctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbookfiller/context/block"
	"go.uber.org/zap"

	clmath "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/math"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbookplugin"

	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

// getFillableOrders returns two amounts on success:
// 1) fillable ask liquidity in quote denom
// 2) fillable bid liquidity in base denom
//
// The strategy to determine fillable amounts:
// - Get all order book orders
// - Get base and quote denom market price
// - Compute current tick from the market price
// - Process active ask orders to compute the amount in quote denoms that can fill them all
// - Process active bid orders to compute the amount in base denoms that can fill them all
// - Return the computed amounts in
//
// Returns error if:
// - Fails to load orer data
// - Fails to get order book pool
// - Fails to get market price for base and quote denoms
// - Fails to compute the current tick from the markat price
// - Fails to compute either of the fillable amounts.
func (o *orderbookFillerIngestPlugin) getFillableOrders(ctx blockctx.BlockCtxI, canonicalOrderbookResult domain.CanonicalOrderBooksResult) (osmomath.Int, osmomath.Int, error) {
	// Get orders for the given order book.
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

	// Get order book pools
	orderBookPool, err := o.poolsUseCase.GetPool(canonicalOrderbookResult.PoolID)
	if err != nil {
		return osmomath.Int{}, osmomath.Int{}, err
	}

	orderBookState := orderBookPool.GetSQSPoolModel().CosmWasmPoolModel.Data.Orderbook

	blockPrices := ctx.GetPrices()

	baseQuoteMarketPrice := blockPrices.GetPriceForDenom(orderBookState.BaseDenom, orderBookState.QuoteDenom)
	if baseQuoteMarketPrice.IsZero() {
		return osmomath.Int{}, osmomath.Int{}, fmt.Errorf("zero price for order book (%d) with base (%s), quote (%s)", canonicalOrderbookResult.PoolID, orderBookState.BaseDenom, orderBookState.QuoteDenom)
	}

	spotPriceScalingFactor, err := o.tokensUseCase.GetSpotPriceScalingFactorByDenom(orderBookState.QuoteDenom, orderBookState.BaseDenom)
	if err != nil {
		o.logger.Error("failed to get spot price scaling factor", zap.Error(err))
		return osmomath.Int{}, osmomath.Int{}, err
	}

	// Scale the price
	baseQuoteMarketPrice.MulMut(osmomath.BigDecFromDec(spotPriceScalingFactor))

	// Create a map from ticks to cumulative total
	tickRemainingLiqMap := make(map[int64]cosmwasmpool.OrderbookTickLiquidity)
	for _, tick := range orderBookState.Ticks {
		tickRemainingLiqMap[tick.TickId] = tick.TickLiquidity
	}

	currentTick, err := clmath.CalculatePriceToTick(baseQuoteMarketPrice)
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
