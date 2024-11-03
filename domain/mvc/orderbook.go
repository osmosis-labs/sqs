package mvc

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type OrderBookUsecase interface {
	// StoreTicks stores the orderbook ticks for a given orderbook pool id.
	ProcessPool(ctx context.Context, pool sqsdomain.PoolI) error

	// GetTicks returns the orderbook ticks for a given orderbook pool id.
	GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool)

	// GetOrder returns all active orderbook orders for a given address.
	GetActiveOrders(ctx context.Context, address string) ([]orderbookdomain.LimitOrder, bool, error)

	// GetActiveOrdersStream returns a channel for streaming limit orderbook orders for a given address.
	// The caller should range over the channel, but note that channel is never closed since there may be multiple
	// sender goroutines.
	GetActiveOrdersStream(ctx context.Context, address string) <-chan orderbookdomain.OrderbookResult

	// CreateFormattedLimitOrder creates a formatted limit order from the given orderbook and order.
	CreateFormattedLimitOrder(orderbook domain.CanonicalOrderBooksResult, order orderbookdomain.Order) (orderbookdomain.LimitOrder, error)

	// GetClaimableOrdersForOrderbook retrieves all claimable orders for a given orderbook.
	// It fetches all ticks for the orderbook, processes each tick to find claimable orders,
	// and returns a combined list of all claimable orders across all ticks.
	GetClaimableOrdersForOrderbook(ctx context.Context, fillThreshold osmomath.Dec, orderbook domain.CanonicalOrderBooksResult) ([]orderbookdomain.ClaimableOrderbook, error)
}
