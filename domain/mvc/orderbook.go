package mvc

import (
	"context"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type OrderBookUsecase interface {
	// StoreTicks stores the orderbook ticks for a given orderbook pool id.
	ProcessPool(ctx context.Context, pool sqsdomain.PoolI) error

	// GetTicks returns the orderbook ticks for a given orderbook pool id.
	GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool)
}
