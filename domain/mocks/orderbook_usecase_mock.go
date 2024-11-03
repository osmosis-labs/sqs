package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
)

var _ mvc.OrderBookUsecase = &OrderbookUsecaseMock{}

// OrderbookUsecaseMock is a mock implementation of the RouterUsecase interface
type OrderbookUsecaseMock struct {
	ProcessPoolFunc                    func(ctx context.Context, pool sqsdomain.PoolI) error
	GetAllTicksFunc                    func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool)
	GetActiveOrdersFunc                func(ctx context.Context, address string) ([]orderbookdomain.LimitOrder, bool, error)
	GetActiveOrdersStreamFunc          func(ctx context.Context, address string) <-chan orderbookdomain.OrderbookResult
	CreateFormattedLimitOrderFunc      func(orderbook domain.CanonicalOrderBooksResult, order orderbookdomain.Order) (orderbookdomain.LimitOrder, error)
	GetClaimableOrdersForOrderbookFunc func(ctx context.Context, fillThreshold osmomath.Dec, orderbook domain.CanonicalOrderBooksResult) ([]orderbookdomain.ClaimableOrderbook, error)
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

func (m *OrderbookUsecaseMock) GetActiveOrdersStream(ctx context.Context, address string) <-chan orderbookdomain.OrderbookResult {
	if m.GetActiveOrdersStreamFunc != nil {
		return m.GetActiveOrdersStreamFunc(ctx, address)
	}
	panic("unimplemented")
}
func (m *OrderbookUsecaseMock) WithCreateFormattedLimitOrder(order orderbookdomain.LimitOrder, err error) {
	m.CreateFormattedLimitOrderFunc = func(domain.CanonicalOrderBooksResult, orderbookdomain.Order) (orderbookdomain.LimitOrder, error) {
		return order, err
	}
}

func (m *OrderbookUsecaseMock) CreateFormattedLimitOrder(orderbook domain.CanonicalOrderBooksResult, order orderbookdomain.Order) (orderbookdomain.LimitOrder, error) {
	if m.CreateFormattedLimitOrderFunc != nil {
		return m.CreateFormattedLimitOrderFunc(orderbook, order)
	}
	panic("unimplemented")
}

func (m *OrderbookUsecaseMock) GetClaimableOrdersForOrderbook(ctx context.Context, fillThreshold osmomath.Dec, orderbook domain.CanonicalOrderBooksResult) ([]orderbookdomain.ClaimableOrderbook, error) {
	if m.GetClaimableOrdersForOrderbookFunc != nil {
		return m.GetClaimableOrdersForOrderbookFunc(ctx, fillThreshold, orderbook)
	}
	panic("unimplemented")
}

func (m *OrderbookUsecaseMock) WithGetClaimableOrdersForOrderbook(orders []orderbookdomain.ClaimableOrderbook, err error) {
	m.GetClaimableOrdersForOrderbookFunc = func(ctx context.Context, fillThreshold osmomath.Dec, orderbook domain.CanonicalOrderBooksResult) ([]orderbookdomain.ClaimableOrderbook, error) {
		return orders, err
	}
}
