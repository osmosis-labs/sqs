package fillbot

import (
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	blockctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/fillbot/context/block"
	"go.uber.org/zap"

	clmath "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/math"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"

	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

var one = osmomath.MustNewDecFromStr("1")

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
	if len(askOrders) == 0 && len(bidOrders) == 0 {
		return osmomath.Int{}, osmomath.Int{}, fmt.Errorf("no orders found for canonical order book id (%d)", canonicalOrderbookResult.PoolID)
	}

	// Get order book pools
	orderBookPool, err := o.poolsUseCase.GetPool(canonicalOrderbookResult.PoolID)
	if err != nil {
		return osmomath.Int{}, osmomath.Int{}, err
	}

	orderBookState := orderBookPool.GetSQSPoolModel().CosmWasmPoolModel.Data.Orderbook

	// Order book spread factor
	spreadFactor := orderBookPool.GetSQSPoolModel().SpreadFactor
	takerFee, err := o.routerUseCase.GetTakerFee(canonicalOrderbookResult.PoolID)
	if err != nil {
		return osmomath.Int{}, osmomath.Int{}, err
	}

	// Only one taker fee for an order book pair.
	if len(takerFee) == 0 {
		return osmomath.Int{}, osmomath.Int{}, fmt.Errorf("taker fee not found for order book %d", canonicalOrderbookResult.PoolID)
	}

	totalFee := spreadFactor.Add(takerFee[0].TakerFee)

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

	// Get current tick from market price
	currentTick, err := clmath.CalculatePriceToTick(baseQuoteMarketPrice)
	if err != nil {
		o.logger.Error("failed to calculate price to tick", zap.Error(err))
		return osmomath.Int{}, osmomath.Int{}, err
	}

	// Get fillable ask amount in quote denom.
	fillableAskAmountInQuoteDenom, err := o.getFillableAskAmountInQuoteDenom(askOrders, currentTick, tickRemainingLiqMap)
	if err != nil {
		fillableAskAmountInQuoteDenom = osmomath.ZeroInt()
		o.logger.Error("failed to get fillable ask amount in quote denom", zap.Error(err), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	// Get fillable bid amount in base denom.
	fillableBidAmountInBaseDenom, err := o.getFillableBidAmountInBaseDenom(bidOrders, currentTick, tickRemainingLiqMap)
	if err != nil {
		fillableBidAmountInBaseDenom = osmomath.ZeroInt()
		o.logger.Error("failed to get fillable bid amount in base denom", zap.Error(err), zap.Uint64("orderbook_id", canonicalOrderbookResult.PoolID))
	}

	return applyFee(fillableAskAmountInQuoteDenom, totalFee), applyFee(fillableBidAmountInBaseDenom, totalFee), nil
}

// applyFee applies the fee to the amount, increasing it.
// amount = amount / (1 - fee)
func applyFee(amount osmomath.Int, fee osmomath.Dec) osmomath.Int {
	amountDec := amount.ToLegacyDec()
	amountDec.QuoRoundupMut(one.Sub(fee))
	return amountDec.Ceil().TruncateInt()
}

// getFillableAskAmountInQuoteDenom returns the fillable ask amount in quote denom.
// Iterates over all ask orders, identifying their ticks.
// Compares the order tick to the current tick. If ask order is below the market tick, it is fillable.
// In that case, we determine the remaining ask liquidity on that tick, applying the price to get its value in the quote denom.
func (o *orderbookFillerIngestPlugin) getFillableAskAmountInQuoteDenom(askOrders []orderbookdomain.Order, currentTick int64, tickRemainingLiqMap map[int64]cosmwasmpool.OrderbookTickLiquidity) (osmomath.Int, error) {
	fillableAskAmountInQuoteDenom := osmomath.ZeroBigDec()

	// Multiple orders may be placed on the same tick, so we need to keep track of which ticks we have processed
	processedTickMap := make(map[int64]struct{})

	for _, order := range askOrders {
		if order.TickId < currentTick {
			// Check if tick has already been processed by another order.
			_, hasProcessedTick := processedTickMap[order.TickId]
			if hasProcessedTick {
				continue
			}

			// Get remaining liquidity on that tick.
			remainingTickLiq, ok := tickRemainingLiqMap[order.TickId]
			if !ok {
				return osmomath.Int{}, fmt.Errorf("ask liquidity not found for tick %d", order.TickId)
			}

			orderAmountAsk := remainingTickLiq.AskLiquidity

			// Get price from tick.
			tickPrice, err := clmath.TickToPrice(order.TickId)
			if err != nil {
				return osmomath.Int{}, err
			}

			// Apply the price to the remaining ask liquidity on the tick to get its value in the quote denom.
			curFillableAskAmountInQuoteDenom := cosmwasmpool.OrderbookValueInOppositeDirection(orderAmountAsk, tickPrice, cosmwasmpool.ASK, cosmwasmpool.ROUND_UP).CeilMut()

			fillableAskAmountInQuoteDenom.AddMut(curFillableAskAmountInQuoteDenom)

			// Update the tick as processed in case there are multiple orders placed on the same tick.
			processedTickMap[order.TickId] = struct{}{}
		}
	}

	// Return the total fillable amount across all orders/ticks.
	return fillableAskAmountInQuoteDenom.Ceil().Dec().TruncateInt(), nil
}

// getFillableBidAmountInBaseDenom returns the fillable bid amount in base denom.
// Iterates over all bid orders, identifying their ticks.
// Compares the order tick to the current tick. If bid order is above the market tick, it is fillable.
// In that case, we determine the remaining bi liquidity on that tick, applying the price to get its value in the base denom.
func (o *orderbookFillerIngestPlugin) getFillableBidAmountInBaseDenom(bidOrders []orderbookdomain.Order, currentTick int64, tickRemainingLiqMap map[int64]cosmwasmpool.OrderbookTickLiquidity) (osmomath.Int, error) {
	fillableBidAmountInBaseDenom := osmomath.ZeroBigDec()

	// Multiple orders may be placed on the same tick, so we need to keep track of which ticks we have processed
	processedTickMap := make(map[int64]struct{})

	for _, order := range bidOrders {
		if order.TickId > currentTick {
			// Check if tick has already been processed by another order.
			_, hasProcessedTick := processedTickMap[order.TickId]
			if hasProcessedTick {
				continue
			}

			// Get remaining liquidity on that tick.
			remainingTickLiq, ok := tickRemainingLiqMap[order.TickId]
			if !ok {
				return osmomath.Int{}, fmt.Errorf("ask liquidity not found for tick %d", order.TickId)
			}

			orderAmountBid := remainingTickLiq.BidLiquidity

			// Get price from tick.
			tickPrice, err := clmath.TickToPrice(order.TickId)
			if err != nil {
				return osmomath.Int{}, err
			}

			// Apply the price to the remaining ask liquidity on the tick to get its value in the quote denom.
			curFillableAskAmountInQuoteDenom := cosmwasmpool.OrderbookValueInOppositeDirection(orderAmountBid, tickPrice, cosmwasmpool.BID, cosmwasmpool.ROUND_UP).CeilMut()

			fillableBidAmountInBaseDenom.AddMut(curFillableAskAmountInQuoteDenom)

			// Update the tick as processed in case there are multiple orders placed on the same tick.
			processedTickMap[order.TickId] = struct{}{}
		}
	}

	return fillableBidAmountInBaseDenom.Ceil().Dec().TruncateInt(), nil
}
