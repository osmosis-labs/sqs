package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

var _ mvc.OrderBookUsecase = &OrderbookUsecaseMock{}

// OrderbookUsecaseMock is a mock implementation of the RouterUsecase interface
type OrderbookUsecaseMock struct {
	ProcessPoolFunc     func(ctx context.Context, pool sqsdomain.PoolI) error
	GetAllTicksFunc     func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool)
	GetActiveOrdersFunc func(ctx context.Context, address string) ([]orderbookdomain.LimitOrder, bool, error)
}

func (m *OrderbookUsecaseMock) ProcessPool(ctx context.Context, pool sqsdomain.PoolI) error {
	if m.ProcessPoolFunc != nil {
		return m.ProcessPoolFunc(ctx, pool)
	}
	panic("unimplemented")
}

func (m *OrderbookUsecaseMock) GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	if m.GetAllTicksFunc != nil {
		return m.GetAllTicksFunc(poolID)
	}
	panic("unimplemented")
}

func (m *OrderbookUsecaseMock) GetActiveOrders(ctx context.Context, address string) ([]orderbookdomain.LimitOrder, bool, error) {
	if m.GetActiveOrdersFunc != nil {
		return m.GetActiveOrdersFunc(ctx, address)
	}
	panic("unimplemented")
}
