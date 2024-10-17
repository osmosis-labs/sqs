package claimbot

import (
	"context"
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/log"

	"go.uber.org/zap"
)

type order struct {
	Orderbook domain.CanonicalOrderBooksResult
	Orders    orderbookdomain.Orders
	Err       error
}

// processOrderbooksAndGetClaimableOrders processes a list of orderbooks and returns claimable orders for each.
func processOrderbooksAndGetClaimableOrders(
	ctx context.Context,
	fillThreshold osmomath.Dec,
	orderbooks []domain.CanonicalOrderBooksResult,
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	orderbookusecase mvc.OrderBookUsecase,
	logger log.Logger,
) []order {
	var result []order
	for _, orderbook := range orderbooks {
		processedOrder := processOrderbook(ctx, fillThreshold, orderbook, orderbookRepository, orderBookClient, orderbookusecase, logger)
		result = append(result, processedOrder)
	}
	return result
}

// processOrderbook processes a single orderbook and returns an order struct containing the processed orderbook and its claimable orders.
func processOrderbook(
	ctx context.Context,
	fillThreshold osmomath.Dec,
	orderbook domain.CanonicalOrderBooksResult,
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	orderbookusecase mvc.OrderBookUsecase,
	logger log.Logger,
) order {
	claimable, err := getClaimableOrdersForOrderbook(ctx, fillThreshold, orderbook, orderbookRepository, orderBookClient, orderbookusecase, logger)
	if err != nil {
		return order{
			Orderbook: orderbook,
			Err:       err,
		}
	}
	return order{
		Orderbook: orderbook,
		Orders:    claimable,
	}
}

// getClaimableOrdersForOrderbook retrieves all claimable orders for a given orderbook.
// It fetches all ticks for the orderbook, processes each tick to find claimable orders,
// and returns a combined list of all claimable orders across all ticks.
func getClaimableOrdersForOrderbook(
	ctx context.Context,
	fillThreshold osmomath.Dec,
	orderbook domain.CanonicalOrderBooksResult,
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	orderbookusecase mvc.OrderBookUsecase,
	logger log.Logger,
) (orderbookdomain.Orders, error) {
	ticks, ok := orderbookRepository.GetAllTicks(orderbook.PoolID)
	if !ok {
		return nil, fmt.Errorf("no ticks for orderbook")
	}

	var claimable orderbookdomain.Orders
	for _, t := range ticks {
		tickClaimable, err := getClaimableOrdersForTick(ctx, fillThreshold, orderbook, t, orderBookClient, orderbookusecase, logger)
		if err != nil {
			logger.Error("error processing tick", zap.String("orderbook", orderbook.ContractAddress), zap.Int64("tick", t.Tick.TickId), zap.Error(err))
			continue
		}
		claimable = append(claimable, tickClaimable...)
	}

	return claimable, nil
}

// getClaimableOrdersForTick retrieves claimable orders for a specific tick in an orderbook
// It processes all ask/bid direction orders and filters the orders that are claimable.
func getClaimableOrdersForTick(
	ctx context.Context,
	fillThreshold osmomath.Dec,
	orderbook domain.CanonicalOrderBooksResult,
	tick orderbookdomain.OrderbookTick,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	orderbookusecase mvc.OrderBookUsecase,
	logger log.Logger,
) (orderbookdomain.Orders, error) {
	orders, err := orderBookClient.GetOrdersByTick(ctx, orderbook.ContractAddress, tick.Tick.TickId)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch orderbook orders by tick ID: %w", err)
	}

	if len(orders) == 0 {
		return nil, nil
	}

	askClaimable := getClaimableOrders(orderbook, orders.OrderByDirection("ask"), tick.TickState.AskValues, fillThreshold, orderbookusecase, logger)
	bidClaimable := getClaimableOrders(orderbook, orders.OrderByDirection("bid"), tick.TickState.BidValues, fillThreshold, orderbookusecase, logger)

	return append(askClaimable, bidClaimable...), nil
}

// getClaimableOrders determines which orders are claimable for a given direction (ask or bid) in a tick.
// If the tick is fully filled, all orders are considered claimable. Otherwise, it filters the orders
// based on the fill threshold.
func getClaimableOrders(
	orderbook domain.CanonicalOrderBooksResult,
	orders orderbookdomain.Orders,
	tickValues orderbookdomain.TickValues,
	fillThreshold osmomath.Dec,
	orderbookusecase mvc.OrderBookUsecase,
	logger log.Logger,
) orderbookdomain.Orders {
	if isTickFullyFilled(tickValues) {
		return orders
	}

	return filterClaimableOrders(orderbook, orders, fillThreshold, orderbookusecase, logger)
}

// isTickFullyFilled checks if a tick is fully filled by comparing its cumulative total value
// to its effective total amount swapped.
func isTickFullyFilled(tickValues orderbookdomain.TickValues) bool {
	if len(tickValues.CumulativeTotalValue) == 0 || len(tickValues.EffectiveTotalAmountSwapped) == 0 {
		return false // empty values, thus not fully filled
	}
	return tickValues.CumulativeTotalValue == tickValues.EffectiveTotalAmountSwapped
}

// filterClaimableOrders processes a list of orders and returns only those that are considered claimable.
func filterClaimableOrders(
	orderbook domain.CanonicalOrderBooksResult,
	orders orderbookdomain.Orders,
	fillThreshold osmomath.Dec,
	orderbookusecase mvc.OrderBookUsecase,
	logger log.Logger,
) orderbookdomain.Orders {
	var claimable orderbookdomain.Orders
	for _, order := range orders {
		if isOrderClaimable(orderbook, order, fillThreshold, orderbookusecase, logger) {
			claimable = append(claimable, order)
		}
	}
	return claimable
}

// isOrderClaimable determines if a single order is claimable based on the fill threshold.
func isOrderClaimable(
	orderbook domain.CanonicalOrderBooksResult,
	order orderbookdomain.Order,
	fillThreshold osmomath.Dec,
	orderbookusecase mvc.OrderBookUsecase,
	logger log.Logger,
) bool {
	result, err := orderbookusecase.CreateFormattedLimitOrder(orderbook, order)
	if err != nil {
		logger.Info(
			"unable to create orderbook limit order; marking as not claimable",
			zap.String("orderbook", orderbook.ContractAddress),
			zap.Int64("order", order.OrderId),
			zap.Error(err),
		)
		return false
	}
	return result.IsClaimable(fillThreshold)
}
