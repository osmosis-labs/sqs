package orderbookplugindomain

import (
	"context"
)

// OrderbookCWAPIClient is an interface for fetching orders by tick from the orderbook contract.
type OrderbookCWAPIClient interface {
	// GetOrdersByTick fetches orders by tick from the orderbook contract.
	GetOrdersByTick(ctx context.Context, contractAddress string, tick int64) ([]Order, error)
}
