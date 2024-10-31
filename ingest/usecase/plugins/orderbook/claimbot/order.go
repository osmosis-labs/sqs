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
	Orders    []orderbookdomain.ClaimableOrderbook
	Error     error
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
			orders, err := orderbookusecase.GetClaimableOrdersForOrderbook(ctx, fillThreshold, orderbook)
			ch <- processedOrderbook{
				Orderbook: orderbook,
				Orders:    orders,
				Error:     err,
			}
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
