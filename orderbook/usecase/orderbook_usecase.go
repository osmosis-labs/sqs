package orderbookusecase

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/orderbook/telemetry"
	"github.com/osmosis-labs/sqs/orderbook/types"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"go.uber.org/zap"

	clmath "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/math"
)

type OrderbookUseCaseImpl struct {
	orderbookRepository orderbookdomain.OrderBookRepository
	orderBookClient     orderbookgrpcclientdomain.OrderBookClient
	poolsUsecease       mvc.PoolsUsecase
	tokensUsecease      mvc.TokensUsecase
	logger              log.Logger
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
	return &OrderbookUseCaseImpl{
		orderbookRepository: orderbookRepository,
		orderBookClient:     orderBookClient,
		poolsUsecease:       poolsUsecease,
		tokensUsecease:      tokensUsecease,
		logger:              logger,
	}
}

// GetAllTicks implements mvc.OrderBookUsecase.
func (o *OrderbookUseCaseImpl) GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	return o.orderbookRepository.GetAllTicks(poolID)
}

// ProcessPool implements mvc.OrderBookUsecase.
func (o *OrderbookUseCaseImpl) ProcessPool(ctx context.Context, pool sqsdomain.PoolI) error {
	if pool == nil {
		return types.PoolNilError{}
	}

	cosmWasmPoolModel := pool.GetSQSPoolModel().CosmWasmPoolModel
	if cosmWasmPoolModel == nil {
		return types.CosmWasmPoolModelNilError{}
	}

	poolID := pool.GetId()
	if !cosmWasmPoolModel.IsOrderbook() {
		return types.NotAnOrderbookPoolError{PoolID: poolID}
	}

	if cosmWasmPoolModel.Data.Orderbook == nil {
		return fmt.Errorf("pool has no orderbook data %d", poolID)
	}

	// Update the orderbook client with the orderbook pool ID.
	ticks := cosmWasmPoolModel.Data.Orderbook.Ticks
	if len(ticks) == 0 {
		return nil // early return, nothing do
	}

	cwModel, ok := pool.GetUnderlyingPool().(*cwpoolmodel.CosmWasmPool)
	if !ok {
		return types.FailedToCastPoolModelError{}
	}

	// Get tick IDs
	tickIDs := make([]int64, 0, len(ticks))
	for _, tick := range ticks {
		tickIDs = append(tickIDs, tick.TickId)
	}

	// Fetch tick states
	tickStates, err := o.orderBookClient.FetchTicks(ctx, maxQueryTicks, cwModel.ContractAddress, tickIDs)
	if err != nil {
		return types.FetchTicksError{ContractAddress: cwModel.ContractAddress, Err: err}
	}

	// Fetch unrealized cancels
	unrealizedCancels, err := o.orderBookClient.FetchTickUnrealizedCancels(ctx, maxQueryTicksCancels, cwModel.ContractAddress, tickIDs)
	if err != nil {
		return types.FetchUnrealizedCancelsError{ContractAddress: cwModel.ContractAddress, Err: err}
	}

	tickDataMap := make(map[int64]orderbookdomain.OrderbookTick, len(ticks))
	for i, tick := range ticks {
		unrealizedCancel := unrealizedCancels[i]

		// Validate the tick IDs match between the tick and the unrealized cancel
		if unrealizedCancel.TickID != tick.TickId {
			return types.TickIDMismatchError{ExpectedID: tick.TickId, ActualID: unrealizedCancel.TickID}
		}

		tickState := tickStates[i]
		if tickState.TickID != tick.TickId {
			return types.TickIDMismatchError{ExpectedID: tick.TickId, ActualID: tickState.TickID}
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

var (
	// fetchActiveOrdersEvery is a duration in which orders are pushed to the client periodically
	// This is an arbitrary number selected to avoid spamming the client
	fetchActiveOrdersDuration = 10 * time.Second

	// getActiveOrdersStreamChanLen is the length of the channel for active orders stream
	// length is arbitrary number selected to avoid blocking
	getActiveOrdersStreamChanLen = 50
)

// GetActiveOrdersStream implements mvc.OrderBookUsecase.
func (o *OrderbookUseCaseImpl) GetActiveOrdersStream(ctx context.Context, address string) <-chan orderbookdomain.OrderbookResult {
	// Result channel
	c := make(chan orderbookdomain.OrderbookResult, getActiveOrdersStreamChanLen)

	// Function to fetch orders
	fetchOrders := func(ctx context.Context) {
		orderbooks, err := o.poolsUsecease.GetAllCanonicalOrderbookPoolIDs()
		if err != nil {
			c <- orderbookdomain.OrderbookResult{
				Error: types.FailedGetAllCanonicalOrderbookPoolIDsError{Err: err},
			}
			return
		}

		for _, orderbook := range orderbooks {
			go func(orderbook domain.CanonicalOrderBooksResult) {
				limitOrders, isBestEffort, err := o.processOrderBookActiveOrders(ctx, orderbook, address)
				if len(limitOrders) == 0 && err == nil {
					return // skip empty orders
				}

				if err != nil {
					telemetry.ProcessingOrderbookActiveOrdersErrorCounter.Inc()
					o.logger.Error(telemetry.ProcessingOrderbookActiveOrdersErrorMetricName, zap.Any("pool_id", orderbook.PoolID), zap.Any("err", err))
				}

				select {
				case c <- orderbookdomain.OrderbookResult{
					PoolID:       orderbook.PoolID,
					IsBestEffort: isBestEffort,
					LimitOrders:  limitOrders,
					Error:        err,
				}:
				case <-ctx.Done():
					return
				}
			}(orderbook)
		}
	}

	// Fetch orders immediately on start
	go fetchOrders(ctx)

	// Pull orders periodically based on duration
	go func() {
		ticker := time.NewTicker(fetchActiveOrdersDuration)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fetchOrders(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()

	return c
}

// GetActiveOrders implements mvc.OrderBookUsecase.
func (o *OrderbookUseCaseImpl) GetActiveOrders(ctx context.Context, address string) ([]orderbookdomain.LimitOrder, bool, error) {
	orderbooks, err := o.poolsUsecease.GetAllCanonicalOrderbookPoolIDs()
	if err != nil {
		return nil, false, types.FailedGetAllCanonicalOrderbookPoolIDsError{Err: err}
	}

	results := make(chan orderbookdomain.OrderbookResult, len(orderbooks))

	// Process orderbooks concurrently
	for _, orderbook := range orderbooks {
		go func(orderbook domain.CanonicalOrderBooksResult) {
			limitOrders, isBestEffort, err := o.processOrderBookActiveOrders(ctx, orderbook, address)
			results <- orderbookdomain.OrderbookResult{
				IsBestEffort: isBestEffort,
				PoolID:       orderbook.PoolID,
				LimitOrders:  limitOrders,
				Error:        err,
			}
		}(orderbook)
	}

	// Collect results
	finalResults := []orderbookdomain.LimitOrder{}
	isBestEffort := false

	for i := 0; i < len(orderbooks); i++ {
		select {
		case result := <-results:
			if result.Error != nil {
				telemetry.ProcessingOrderbookActiveOrdersErrorCounter.Inc()
				o.logger.Error(telemetry.ProcessingOrderbookActiveOrdersErrorMetricName, zap.Any("pool_id", result.PoolID), zap.Any("err", result.Error))
			}

			isBestEffort = isBestEffort || result.IsBestEffort

			finalResults = append(finalResults, result.LimitOrders...)
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
func (o *OrderbookUseCaseImpl) processOrderBookActiveOrders(ctx context.Context, orderbook domain.CanonicalOrderBooksResult, ownerAddress string) ([]orderbookdomain.LimitOrder, bool, error) {
	if err := orderbook.Validate(); err != nil {
		return nil, false, err
	}

	orders, count, err := o.orderBookClient.GetActiveOrders(ctx, orderbook.ContractAddress, ownerAddress)
	if err != nil {
		return nil, false, types.FailedToGetActiveOrdersError{
			ContractAddress: orderbook.ContractAddress,
			OwnerAddress:    ownerAddress,
			Err:             err,
		}
	}

	// There are orders to process for given orderbook
	if count == 0 {
		return nil, false, nil
	}

	// Create a slice to store the results
	results := make([]orderbookdomain.LimitOrder, 0, len(orders))

	// If we encounter
	isBestEffort := false

	// For each order, create a formatted limit order
	for _, order := range orders {
		// create limit order
		result, err := o.CreateFormattedLimitOrder(
			orderbook,
			order,
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

// ZeroDec is a zero decimal value.
// It is defined in a global space to avoid creating a new instance every time.
var zeroDec = osmomath.ZeroDec()

// CreateFormattedLimitOrder creates a limit order from the orderbook order.
func (o *OrderbookUseCaseImpl) CreateFormattedLimitOrder(orderbook domain.CanonicalOrderBooksResult, order orderbookdomain.Order) (orderbookdomain.LimitOrder, error) {
	quoteToken, err := o.tokensUsecease.GetMetadataByChainDenom(orderbook.Quote)
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.FailedToGetMetadataError{
			TokenDenom: orderbook.Quote,
			Err:        err,
		}
	}

	quoteAsset := orderbookdomain.Asset{
		Symbol:   quoteToken.CoinMinimalDenom,
		Decimals: quoteToken.Precision,
	}

	baseToken, err := o.tokensUsecease.GetMetadataByChainDenom(orderbook.Base)
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.FailedToGetMetadataError{
			TokenDenom: orderbook.Base,
			Err:        err,
		}
	}

	baseAsset := orderbookdomain.Asset{
		Symbol:   baseToken.CoinMinimalDenom,
		Decimals: baseToken.Precision,
	}

	tickForOrder, ok := o.orderbookRepository.GetTickByID(orderbook.PoolID, order.TickId)
	if !ok {
		telemetry.GetTickByIDNotFoundCounter.Inc()
		return orderbookdomain.LimitOrder{}, types.TickForOrderbookNotFoundError{
			OrderbookAddress: orderbook.ContractAddress,
			TickID:           order.TickId,
		}
	}

	tickState := tickForOrder.TickState
	unrealizedCancels := tickForOrder.UnrealizedCancels

	quantity, err := osmomath.NewDecFromStr(order.Quantity)
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.ParsingQuantityError{
			Quantity: order.Quantity,
			Err:      err,
		}
	}

	placedQuantity, err := osmomath.NewDecFromStr(order.PlacedQuantity)
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.ParsingPlacedQuantityError{
			PlacedQuantity: order.PlacedQuantity,
			Err:            err,
		}
	}

	if placedQuantity.Equal(zeroDec) || placedQuantity.LT(zeroDec) {
		return orderbookdomain.LimitOrder{}, types.InvalidPlacedQuantityError{PlacedQuantity: placedQuantity}
	}

	// Calculate percent claimed
	percentClaimed := placedQuantity.Sub(quantity).Quo(placedQuantity)

	// Calculate normalization factor for price
	normalizationFactor, err := o.tokensUsecease.GetSpotPriceScalingFactorByDenom(baseAsset.Symbol, quoteAsset.Symbol)
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.GettingSpotPriceScalingFactorError{
			BaseDenom:  baseAsset.Symbol,
			QuoteDenom: quoteAsset.Symbol,
			Err:        err,
		}
	}

	// Determine tick values and unrealized cancels based on order direction
	var tickEtas, tickUnrealizedCancelled osmomath.Dec
	if order.OrderDirection == "bid" {
		tickEtas, err = osmomath.NewDecFromStr(tickState.BidValues.EffectiveTotalAmountSwapped)
		if err != nil {
			return orderbookdomain.LimitOrder{}, types.ParsingTickValuesError{
				Field: "EffectiveTotalAmountSwapped (bid)",
				Err:   err,
			}
		}

		if unrealizedCancels.BidUnrealizedCancels.IsNil() {
			return orderbookdomain.LimitOrder{}, types.ParsingUnrealizedCancelsError{
				Field: "BidUnrealizedCancels",
				Err:   fmt.Errorf("nil value for bid unrealized cancels"),
			}
		}

		tickUnrealizedCancelled = osmomath.NewDecFromInt(unrealizedCancels.BidUnrealizedCancels)
	} else {
		tickEtas, err = osmomath.NewDecFromStr(tickState.AskValues.EffectiveTotalAmountSwapped)
		if err != nil {
			return orderbookdomain.LimitOrder{}, types.ParsingTickValuesError{
				Field: "EffectiveTotalAmountSwapped (ask)",
				Err:   err,
			}
		}

		if unrealizedCancels.AskUnrealizedCancels.IsNil() {
			return orderbookdomain.LimitOrder{}, types.ParsingUnrealizedCancelsError{
				Field: "AskUnrealizedCancels",
				Err:   fmt.Errorf("nil value for ask unrealized cancels"),
			}
		}

		tickUnrealizedCancelled = osmomath.NewDecFromInt(unrealizedCancels.AskUnrealizedCancels)
	}

	// Calculate total ETAs and total filled
	etas, err := osmomath.NewDecFromStr(order.Etas)
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.ParsingEtasError{
			Etas: order.Etas,
			Err:  err,
		}
	}

	tickTotalEtas := tickEtas.Add(tickUnrealizedCancelled)

	totalFilled := osmomath.MaxDec(
		tickTotalEtas.Sub(etas.Sub(placedQuantity.Sub(quantity))),
		osmomath.ZeroDec(),
	)

	// Calculate percent filled using
	percentFilled := osmomath.MinDec(
		totalFilled.Quo(placedQuantity),
		osmomath.OneDec(),
	)

	// Determine order status based on percent filled
	status, err := order.Status(percentFilled.MustFloat64())
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.MappingOrderStatusError{Err: err}
	}

	// Calculate price based on tick ID
	price, err := clmath.TickToPrice(order.TickId)
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.ConvertingTickToPriceError{TickID: order.TickId, Err: err}
	}

	// Calculate output based on order direction
	var output osmomath.Dec
	if order.OrderDirection == "bid" {
		output = placedQuantity.Quo(price.Dec())
	} else {
		output = placedQuantity.Mul(price.Dec())
	}

	// Calculate normalized price
	normalizedPrice := price.Dec().Mul(normalizationFactor)

	// Convert placed_at to a nano second timestamp
	placedAt, err := strconv.ParseInt(order.PlacedAt, 10, 64)
	if err != nil {
		return orderbookdomain.LimitOrder{}, types.ParsingPlacedAtError{
			PlacedAt: order.PlacedAt,
			Err:      err,
		}
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
		PercentClaimed:   percentClaimed,
		TotalFilled:      totalFilled,
		PercentFilled:    percentFilled,
		OrderbookAddress: orderbook.ContractAddress,
		Price:            normalizedPrice,
		Status:           status,
		Output:           output,
		QuoteAsset:       quoteAsset,
		BaseAsset:        baseAsset,
		PlacedAt:         placedAt,
	}, nil
}

func (o *OrderbookUseCaseImpl) GetClaimableOrdersForOrderbook(ctx context.Context, fillThreshold osmomath.Dec, orderbook domain.CanonicalOrderBooksResult) ([]orderbookdomain.ClaimableOrderbook, error) {
	ticks, ok := o.orderbookRepository.GetAllTicks(orderbook.PoolID)
	if !ok {
		return nil, fmt.Errorf("no ticks found for orderbook %s with pool %d", orderbook.ContractAddress, orderbook.PoolID)
	}

	var orders []orderbookdomain.ClaimableOrderbook
	for _, tick := range ticks {
		tickOrders, err := o.getClaimableOrdersForTick(ctx, fillThreshold, orderbook, tick)
		orders = append(orders, orderbookdomain.ClaimableOrderbook{
			Tick:   tick,
			Orders: tickOrders,
			Error:  err,
		})
	}

	return orders, nil
}

// getClaimableOrdersForTick retrieves claimable orders for a specific tick in an orderbook
// It processes all ask/bid direction orders and filters the orders that are claimable.
func (o *OrderbookUseCaseImpl) getClaimableOrdersForTick(
	ctx context.Context,
	fillThreshold osmomath.Dec,
	orderbook domain.CanonicalOrderBooksResult,
	tick orderbookdomain.OrderbookTick,
) ([]orderbookdomain.ClaimableOrder, error) {
	orders, err := o.orderBookClient.GetOrdersByTick(ctx, orderbook.ContractAddress, tick.Tick.TickId)
	if err != nil {
		return nil, err
	}

	if len(orders) == 0 {
		return nil, nil // nothing to process
	}

	askClaimable, err := o.getClaimableOrders(orderbook, orders.OrderByDirection("ask"), tick.TickState.AskValues, fillThreshold)
	if err != nil {
		return nil, err
	}

	bidClaimable, err := o.getClaimableOrders(orderbook, orders.OrderByDirection("bid"), tick.TickState.BidValues, fillThreshold)
	if err != nil {
		return nil, err
	}

	return append(askClaimable, bidClaimable...), nil
}

// getClaimableOrders determines which orders are claimable for a given direction (ask or bid) in a tick.
// If the tick is fully filled, all orders are considered claimable. Otherwise, it filters the orders
// based on the fill threshold.
func (o *OrderbookUseCaseImpl) getClaimableOrders(
	orderbook domain.CanonicalOrderBooksResult,
	orders orderbookdomain.Orders,
	tickValues orderbookdomain.TickValues,
	fillThreshold osmomath.Dec,
) ([]orderbookdomain.ClaimableOrder, error) {
	isFilled, err := tickValues.IsTickFullyFilled()
	if err != nil {
		return nil, err
	}

	var result []orderbookdomain.ClaimableOrder
	for _, order := range orders {
		if isFilled {
			result = append(result, orderbookdomain.ClaimableOrder{Order: order})
			continue
		}
		claimable, err := o.isOrderClaimable(orderbook, order, fillThreshold)
		orderToAdd := orderbookdomain.ClaimableOrder{Order: order, Error: err}

		if err != nil || claimable {
			result = append(result, orderToAdd)
		}
	}

	return result, nil
}

// isOrderClaimable determines if a single order is claimable based on the fill threshold.
func (o *OrderbookUseCaseImpl) isOrderClaimable(
	orderbook domain.CanonicalOrderBooksResult,
	order orderbookdomain.Order,
	fillThreshold osmomath.Dec,
) (bool, error) {
	result, err := o.CreateFormattedLimitOrder(orderbook, order)
	if err != nil {
		return false, err
	}
	return result.IsClaimable(fillThreshold), nil
}
