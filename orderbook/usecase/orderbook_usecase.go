package orderbookusecase

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/orderbook/telemetry"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"go.uber.org/zap"

	clmath "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/math"
)

type OrderbookUseCaseImpl struct {
	orderbookRepository orderbookdomain.OrderBookRepository
	orderBookClient     orderbookgrpcclientdomain.OrderBookClient
	poolsUsecease       mvc.PoolsUsecase
	tokensUsecease      mvc.TokensUsecase
	logger              log.Logger

	processOrderBookActiveOrdersFunc func(ctx context.Context, orderBook domain.CanonicalOrderBooksResult, ownerAddress string) ([]orderbookdomain.LimitOrder, bool, error)
}

var _ mvc.OrderBookUsecase = &OrderbookUseCaseImpl{}

const (
	// Max number of ticks to query at a time
	maxQueryTicks = 500
	// Max number of ticks cancels to query at a time
	maxQueryTicksCancels = 100
)

// New creates a new orderbook use case.
func New(
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	poolsUsecease mvc.PoolsUsecase,
	tokensUsecease mvc.TokensUsecase,
	logger log.Logger,
) *OrderbookUseCaseImpl {
	res := OrderbookUseCaseImpl{
		orderbookRepository: orderbookRepository,
		orderBookClient:     orderBookClient,
		poolsUsecease:       poolsUsecease,
		tokensUsecease:      tokensUsecease,
		logger:              logger,
	}

	res.processOrderBookActiveOrdersFunc = res.processOrderBookActiveOrders

	return &res
}

// GetTicks implements mvc.OrderBookUsecase.
func (o *OrderbookUseCaseImpl) GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	return o.orderbookRepository.GetAllTicks(poolID)
}

// StoreTicks implements mvc.OrderBookUsecase.
func (o *OrderbookUseCaseImpl) ProcessPool(ctx context.Context, pool sqsdomain.PoolI) error {
	if pool == nil {
		return fmt.Errorf("pool is nil when processing order book")
	}

	cosmWasmPoolModel := pool.GetSQSPoolModel().CosmWasmPoolModel
	if cosmWasmPoolModel == nil {
		return fmt.Errorf("cw pool model is nil when processing order book")
	}

	poolID := pool.GetId()
	if !cosmWasmPoolModel.IsOrderbook() {
		return fmt.Errorf("pool is not an orderbook pool %d", poolID)
	}

	// Update the orderbook client with the orderbook pool ID.
	ticks := cosmWasmPoolModel.Data.Orderbook.Ticks
	if len(ticks) == 0 {
		return nil // early return, nothing do
	}

	cwModel, ok := pool.GetUnderlyingPool().(*cwpoolmodel.CosmWasmPool)
	if !ok {
		return fmt.Errorf("failed to cast pool model to CosmWasmPool")
	}

	// Get tick IDs
	tickIDs := make([]int64, 0, len(ticks))
	for _, tick := range ticks {
		tickIDs = append(tickIDs, tick.TickId)
	}

	// Fetch tick states
	tickStates, err := o.orderBookClient.FetchTicks(ctx, maxQueryTicks, cwModel.ContractAddress, tickIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch ticks for pool %s: %w", cwModel.ContractAddress, err)
	}

	// Fetch unrealized cancels
	unrealizedCancels, err := o.orderBookClient.FetchTickUnrealizedCancels(ctx, maxQueryTicksCancels, cwModel.ContractAddress, tickIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch unrealized cancels for pool %s: %w", cwModel.ContractAddress, err)
	}

	tickDataMap := make(map[int64]orderbookdomain.OrderbookTick, len(ticks))
	for i, tick := range ticks {
		unrealizedCancel := unrealizedCancels[i]

		// Validate the tick IDs match between the tick and the unrealized cancel
		if unrealizedCancel.TickID != tick.TickId {
			return fmt.Errorf("tick id mismatch when fetching unrealized ticks %d %d", unrealizedCancel.TickID, tick.TickId)
		}

		tickState := tickStates[i]
		if tickState.TickID != tick.TickId {
			return fmt.Errorf("tick id mismatch when fetching tick states %d %d", tickState.TickID, tick.TickId)
		}

		// Update tick map for the pool
		tickDataMap[tick.TickId] = orderbookdomain.OrderbookTick{
			Tick:              &ticks[i],
			TickState:         tickState.TickState,
			UnrealizedCancels: unrealizedCancel.UnrealizedCancelsState,
		}
	}

	// Store the ticks
	o.orderbookRepository.StoreTicks(poolID, tickDataMap)

	return nil
}

// GetActiveOrders implements mvc.OrderBookUsecase.
func (o *OrderbookUseCaseImpl) GetActiveOrders(ctx context.Context, address string) ([]orderbookdomain.LimitOrder, bool, error) {
	orderbooks, err := o.poolsUsecease.GetAllCanonicalOrderbookPoolIDs()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get all canonical orderbook pool IDs: %w", err)
	}

	type orderbookResult struct {
		isBestEffort bool
		orderbookID  uint64
		limitOrders  []orderbookdomain.LimitOrder
		err          error
	}

	results := make(chan orderbookResult, len(orderbooks))

	// Process orderbooks concurrently
	for _, orderbook := range orderbooks {
		go func(orderbook domain.CanonicalOrderBooksResult) {
			limitOrders, isBestEffort, err := o.processOrderBookActiveOrdersFunc(ctx, orderbook, address)

			results <- orderbookResult{
				isBestEffort: isBestEffort,
				orderbookID:  orderbook.PoolID,
				limitOrders:  limitOrders,
				err:          err,
			}
		}(orderbook)
	}

	// Collect results
	finalResults := []orderbookdomain.LimitOrder{}
	isBestEffort := false

	for i := 0; i < len(orderbooks); i++ {
		select {
		case result := <-results:
			if result.err != nil {
				telemetry.ProcessingOrderbookActiveOrdersErrorCounter.Inc()
				o.logger.Error(telemetry.ProcessingOrderbookActiveOrdersErrorMetricName, zap.Any("orderbook_id", result.orderbookID), zap.Any("err", result.err))
				return nil, false, result.err
			}

			isBestEffort = isBestEffort || result.isBestEffort

			finalResults = append(finalResults, result.limitOrders...)
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}

	return finalResults, isBestEffort, nil
}

// processOrderBookActiveOrders fetches and processes the active orders for a given orderbook.
// It returns the active formatted limit orders and an error if any.
// Errors if:
// - failed to fetch active orders
// - failed to fetch metadata by chain denom
// - failed to create limit order
//
// For every order, if an error occurs processing the order, it is skipped rather than failing the entire process.
// This is a best-effort process.
func (o *OrderbookUseCaseImpl) processOrderBookActiveOrders(ctx context.Context, orderBook domain.CanonicalOrderBooksResult, ownerAddress string) ([]orderbookdomain.LimitOrder, bool, error) {
	orders, count, err := o.orderBookClient.GetActiveOrders(ctx, orderBook.ContractAddress, ownerAddress)
	if err != nil {
		return nil, false, err
	}

	// There are orders to process for given orderbook
	if count == 0 {
		return nil, false, nil
	}

	quoteToken, err := o.tokensUsecease.GetMetadataByChainDenom(orderBook.Quote)
	if err != nil {
		return nil, false, err
	}

	baseToken, err := o.tokensUsecease.GetMetadataByChainDenom(orderBook.Base)
	if err != nil {
		return nil, false, err
	}

	// Create a slice to store the results
	results := make([]orderbookdomain.LimitOrder, 0, len(orders))

	// If we encounter
	isBestEffort := false

	// For each order, create a formatted limit order
	for _, order := range orders {
		// create limit order
		result, err := o.createFormattedLimitOrder(
			orderBook.PoolID,
			order,
			orderbookdomain.Asset{
				Symbol:   quoteToken.CoinMinimalDenom,
				Decimals: quoteToken.Precision,
			},
			orderbookdomain.Asset{
				Symbol:   baseToken.CoinMinimalDenom,
				Decimals: baseToken.Precision,
			},
			orderBook.ContractAddress,
		)
		if err != nil {
			telemetry.CreateLimitOrderErrorCounter.Inc()
			o.logger.Error(telemetry.CreateLimitOrderErrorMetricName, zap.Any("order", order), zap.Any("err", err))

			isBestEffort = true

			continue
		}

		results = append(results, result)
	}

	return results, isBestEffort, nil
}

// createFormattedLimitOrder creates a limit order from the orderbook order.
func (o *OrderbookUseCaseImpl) createFormattedLimitOrder(
	poolID uint64,
	order orderbookdomain.Order,
	quoteAsset orderbookdomain.Asset,
	baseAsset orderbookdomain.Asset,
	orderbookAddress string,
) (orderbookdomain.LimitOrder, error) {
	tickForOrder, ok := o.orderbookRepository.GetTickByID(poolID, order.TickId)
	if !ok {
		telemetry.GetTickByIDNotFoundCounter.Inc()
		return orderbookdomain.LimitOrder{}, fmt.Errorf("tick not found %s, %d", orderbookAddress, order.TickId)
	}

	tickState := tickForOrder.TickState
	unrealizedCancels := tickForOrder.UnrealizedCancels

	// Parse quantity as int64
	quantity, err := strconv.ParseInt(order.Quantity, 10, 64)
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing quantity: %w", err)
	}

	// Convert quantity to decimal for the calculations
	quantityDec := osmomath.NewDec(quantity)

	placedQuantity, err := strconv.ParseInt(order.PlacedQuantity, 10, 64)
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing placed quantity: %w", err)
	}

	if placedQuantity == 0 || placedQuantity < 0 {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("placed quantity is 0 or negative")
	}

	placedQuantityDec, err := osmomath.NewDecFromStr(order.PlacedQuantity)
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing placed quantity: %w", err)
	}

	// Calculate percent claimed
	percentClaimed := placedQuantityDec.Sub(quantityDec).Quo(placedQuantityDec)

	// Calculate normalization factor for price
	normalizationFactor, err := o.tokensUsecease.GetSpotPriceScalingFactorByDenom(baseAsset.Symbol, quoteAsset.Symbol)
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("error getting spot price scaling factor: %w", err)
	}

	// Determine tick values and unrealized cancels based on order direction
	var tickEtas, tickUnrealizedCancelled int64
	if order.OrderDirection == "bid" {
		tickEtas, err = strconv.ParseInt(tickState.BidValues.EffectiveTotalAmountSwapped, 10, 64)
		if err != nil {
			return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing bid effective total amount swapped: %w", err)
		}

		tickUnrealizedCancelled, err = strconv.ParseInt(unrealizedCancels.BidUnrealizedCancels.String(), 10, 64)
		if err != nil {
			return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing bid unrealized cancels: %w", err)
		}
	} else {
		tickEtas, err = strconv.ParseInt(tickState.AskValues.EffectiveTotalAmountSwapped, 10, 64)
		if err != nil {
			return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing ask effective total amount swapped: %w", err)
		}

		tickUnrealizedCancelled, err = strconv.ParseInt(unrealizedCancels.AskUnrealizedCancels.String(), 10, 64)
		if err != nil {
			return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing ask unrealized cancels: %w", err)
		}
	}

	// Calculate total ETAs and total filled

	etas, err := strconv.ParseInt(order.Etas, 10, 64)
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing etas: %w", err)
	}

	tickTotalEtas := tickEtas + tickUnrealizedCancelled

	totalFilled := int64(math.Max(
		float64(tickTotalEtas-(etas-(placedQuantity-quantity))),
		0,
	))

	// Calculate percent filled using
	percentFilled, err := osmomath.NewDecFromStr(strconv.FormatFloat(math.Min(float64(totalFilled)/float64(placedQuantity), 1), 'f', -1, 64))
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("error calculating percent filled: %w", err)
	}

	// Determine order status based on percent filled
	status, err := order.Status(percentFilled.MustFloat64())
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("mapping order status: %w", err)
	}

	// Calculate price based on tick ID
	price, err := clmath.TickToPrice(order.TickId)
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("converting tick to price: %w", err)
	}

	// Calculate output based on order direction
	var output osmomath.Dec
	if order.OrderDirection == "bid" {
		output = placedQuantityDec.Quo(price.Dec())
	} else {
		output = placedQuantityDec.Mul(price.Dec())
	}

	// Calculate normalized price
	normalizedPrice := price.Dec().Mul(normalizationFactor)

	// Convert placed_at to a nano second timestamp
	placedAt, err := strconv.ParseInt(order.PlacedAt, 10, 64)
	if err != nil {
		return orderbookdomain.LimitOrder{}, fmt.Errorf("error parsing placed_at: %w", err)
	}
	placedAt = time.Unix(0, placedAt).Unix()

	// Return the mapped limit order
	return orderbookdomain.LimitOrder{
		TickId:           order.TickId,
		OrderId:          order.OrderId,
		OrderDirection:   order.OrderDirection,
		Owner:            order.Owner,
		Quantity:         quantity,
		Etas:             order.Etas,
		ClaimBounty:      order.ClaimBounty,
		PlacedQuantity:   placedQuantity,
		PercentClaimed:   percentClaimed.String(),
		TotalFilled:      totalFilled,
		PercentFilled:    percentFilled.String(),
		OrderbookAddress: orderbookAddress,
		Price:            normalizedPrice.String(),
		Status:           status,
		Output:           output.String(),
		QuoteAsset:       quoteAsset,
		BaseAsset:        baseAsset,
		PlacedAt:         placedAt,
	}, nil
}
