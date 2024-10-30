package claimbot

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

// processedOrderbook is a data structure
// containing the processed orderbook and its claimable orders.
type processedOrderbook struct {
	Orderbook domain.CanonicalOrderBooksResult
	Orders    orderbookdomain.Orders
	Err       error
}

// processOrderbooksAndGetClaimableOrders processes a list of orderbooks and returns claimable orders for each.
// Under the hood processing of each orderbook in done concurrently to speed up the process.
func processOrderbooksAndGetClaimableOrders(
	ctx context.Context,
	orderbookusecase mvc.OrderBookUsecase,
	fillThreshold osmomath.Dec,
	orderbooks []domain.CanonicalOrderBooksResult,
) ([]processedOrderbook, error) {
	ch := make(chan processedOrderbook, len(orderbooks))

	for _, orderbook := range orderbooks {
		go func(orderbook domain.CanonicalOrderBooksResult) {
			o := processOrderbook(ctx, orderbookusecase, fillThreshold, orderbook)
			ch <- o
		}(orderbook)
	}

	var results []processedOrderbook
	for i := 0; i < len(orderbooks); i++ {
		select {
		case result := <-ch:
			results = append(results, result)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return results, nil
}

// processOrderbook processes a single orderbook and returns an order struct containing the processed orderbook and its claimable orders.
func processOrderbook(
	ctx context.Context,
	orderbookusecase mvc.OrderBookUsecase,
	fillThreshold osmomath.Dec,
	orderbook domain.CanonicalOrderBooksResult,
) processedOrderbook {
	claimable, err := orderbookusecase.GetClaimableOrdersForOrderbook(ctx, fillThreshold, orderbook)
	if err != nil {
		return processedOrderbook{
			Orderbook: orderbook,
			Err:       err,
		}
	}
	return processedOrderbook{
		Orderbook: orderbook,
		Orders:    claimable,
	}
}
