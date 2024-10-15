package orderbookplugindomain

import (
	"context"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

// OrderbookCWAPIClient is an interface for fetching orders by tick from the orderbook contract.
type OrderbookCWAPIClient interface {
	// GetOrdersByTick fetches orders by tick from the orderbook contract.
	GetOrdersByTick(ctx context.Context, contractAddress string, tick int64) (orderbookdomain.Orders, error)
}
