package orderbookusecase

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"

	clmath "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/math"
)

type orderbookUseCaseImpl struct {
	orderbookRepository orderbookdomain.OrderBookRepository
	orderBookClient     orderbookgrpcclientdomain.OrderBookClient
	logger              log.Logger
}

var _ mvc.OrderBookUsecase = &orderbookUseCaseImpl{}

func New(orderbookRepository orderbookdomain.OrderBookRepository, orderBookClient orderbookgrpcclientdomain.OrderBookClient, logger log.Logger) mvc.OrderBookUsecase {
	return &orderbookUseCaseImpl{
		orderbookRepository: orderbookRepository,
		orderBookClient:     orderBookClient,
		logger:              logger,
	}
}

// GetTicks implements mvc.OrderBookUsecase.
func (o *orderbookUseCaseImpl) GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	return o.orderbookRepository.GetAllTicks(poolID)
}

// StoreTicks implements mvc.OrderBookUsecase.
func (o *orderbookUseCaseImpl) ProcessPool(ctx context.Context, pool sqsdomain.PoolI) error {
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

	if len(ticks) > 0 {
		cwModel, ok := pool.GetUnderlyingPool().(*cwpoolmodel.CosmWasmPool)
		if !ok {
			return fmt.Errorf("failed to cast pool model to CosmWasmPool")
		}

		// Get tick IDs
		tickIDs := make([]int64, 0, len(ticks))
		for _, tick := range ticks {
			tickIDs = append(tickIDs, tick.TickId)
		}

		unrealizedCancels, err := o.orderBookClient.GetTickUnrealizedCancels(ctx, cwModel.ContractAddress, tickIDs)
		if err != nil {
			return fmt.Errorf("failed to fetch unrealized cancels for ticks %v: %w", tickIDs, err)
		}

		tickDataMap := make(map[int64]orderbookdomain.OrderbookTick, len(ticks))
		for i, tick := range ticks {
			unrealizedCancel := unrealizedCancels[i]

			// Validate the tick IDs match between the tick and the unrealized cancel
			if unrealizedCancel.TickID != tick.TickId {
				return fmt.Errorf("tick id mismatch when fetching unrealized ticks %d %d", unrealizedCancel.TickID, tick.TickId)
			}

			// Update tick map for the pool
			tickDataMap[tick.TickId] = orderbookdomain.OrderbookTick{
				Tick:              &ticks[i],
				UnrealizedCancels: unrealizedCancel.UnrealizedCancelsState,
			}
		}

		// Store the ticks
		o.orderbookRepository.StoreTicks(poolID, tickDataMap)
	}

	return nil
}

// TransformOrder transforms an order into a mapped limit order.
func TransformOrder(
	order orderbookdomain.Order,
	tickStates []orderbookdomain.Tick,
	unrealizedCancels []orderbookgrpcclientdomain.UnrealizedTickCancels,
	quoteAsset orderbookdomain.Asset,
	baseAsset orderbookdomain.Asset,
	orderbookAddress string,
) (orderbookdomain.MappedLimitOrder, error) {
	var orderTickState orderbookdomain.Tick
	for _, tick := range tickStates {
		if tick.TickID == order.TickId {
			orderTickState = tick
			break
		}
	}

	var orderUnrealizedCancels orderbookgrpcclientdomain.UnrealizedTickCancels
	for _, unrealizedCancel := range unrealizedCancels {
		if unrealizedCancel.TickID == order.TickId {
			orderUnrealizedCancels = unrealizedCancel
			break
		}
	}

	// Parse quantity as int64
	quantity, err := strconv.ParseInt(order.Quantity, 10, 64)
	if err != nil {
		return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing quantity: %w", err)
	}

	// Convert quantity to decimal for the calculations
	quantityDec := osmomath.NewDec(quantity)

	placedQuantity, err := strconv.ParseInt(order.PlacedQuantity, 10, 64)
	if err != nil {
		return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing placed quantity: %w", err)
	}

	placedQuantityDec, err := osmomath.NewDecFromStr(order.PlacedQuantity)
	if err != nil {
		return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing placed quantity: %w", err)
	}

	// Calculate percent claimed
	percentClaimed := placedQuantityDec.Sub(quantityDec).Quo(placedQuantityDec)

	// Calculate normalization factor for price
	normalizationFactor := osmomath.NewDec(10).Power(uint64(quoteAsset.Decimals - baseAsset.Decimals))

	// Determine tick values and unrealized cancels based on order direction
	var tickEtas, tickUnrealizedCancelled int64
	if order.OrderDirection == "bid" {
		tickEtas, err = strconv.ParseInt(orderTickState.TickState.BidValues.EffectiveTotalAmountSwapped, 10, 64)
		if err != nil {
			return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing bid effective total amount swapped: %w", err)
		}

		tickUnrealizedCancelled, err = strconv.ParseInt(orderUnrealizedCancels.UnrealizedCancelsState.BidUnrealizedCancels.String(), 10, 64)
		if err != nil {
			return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing bid unrealized cancels: %w", err)
		}
	} else {
		tickEtas, err = strconv.ParseInt(orderTickState.TickState.AskValues.EffectiveTotalAmountSwapped, 10, 64)
		if err != nil {
			return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing ask effective total amount swapped: %w", err)
		}

		tickUnrealizedCancelled, err = strconv.ParseInt(orderUnrealizedCancels.UnrealizedCancelsState.AskUnrealizedCancels.String(), 10, 64)
		if err != nil {
			return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing ask unrealized cancels: %w", err)
		}
	}

	// Calculate total ETAs and total filled

	etas, err := strconv.ParseInt(order.Etas, 10, 64)
	if err != nil {
		return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing etas: %w", err)
	}

	tickTotalEtas := tickEtas + tickUnrealizedCancelled

	totalFilled := int64(math.Max(
		float64(tickTotalEtas-(etas-(placedQuantity-quantity))),
		0,
	))

	// Calculate percent filled using
	percentFilled, err := osmomath.NewDecFromStr(strconv.FormatFloat(math.Min(float64(totalFilled)/float64(placedQuantity), 1), 'f', -1, 64))
	if err != nil {
		return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error calculating percent filled: %w", err)
	}

	// Determine order status based on percent filled
	status, err := order.Status(percentFilled.MustFloat64())
	if err != nil {
		return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("mapping order status: %w", err)
	}

	// Calculate price based on tick ID
	price, err := clmath.TickToPrice(order.TickId)
	if err != nil {
		return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("converting tick to price: %w", err)
	}

	// Calculate output based on order direction
	var output osmomath.Dec
	if order.OrderDirection == "bid" {
		output = placedQuantityDec.Quo(price.Dec())
	} else {
		output = placedQuantityDec.Mul(price.Dec())
	}

	// Calculate normalized price
	normalizedPrice := price.Dec().Quo(normalizationFactor)

	// Convert placed_at to a nano second timestamp
	placedAt, err := strconv.ParseInt(order.PlacedAt, 10, 64)
	if err != nil {
		return orderbookdomain.MappedLimitOrder{}, fmt.Errorf("error parsing placed_at: %w", err)
	}
	placedAt = time.Unix(0, placedAt).Unix()

	// Return the mapped limit order
	return orderbookdomain.MappedLimitOrder{
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
