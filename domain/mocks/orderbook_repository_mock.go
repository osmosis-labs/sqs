package mocks

import (
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

var _ orderbookdomain.OrderBookRepository = &OrderbookRepositoryMock{}

// OrderbookRepositoryMock is a mock implementation of the OrderBookRepository interface.
type OrderbookRepositoryMock struct {
	StoreTicksFunc  func(poolID uint64, ticksMap map[int64]orderbookdomain.OrderbookTick)
	GetAllTicksFunc func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool)
	GetTicksFunc    func(poolID uint64, tickIDs []int64) (map[int64]orderbookdomain.OrderbookTick, error)
	GetTickByIDFunc func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool)
}

// StoreTicks implements OrderBookRepository.
func (m *OrderbookRepositoryMock) StoreTicks(poolID uint64, ticksMap map[int64]orderbookdomain.OrderbookTick) {
	if m.StoreTicksFunc != nil {
		m.StoreTicksFunc(poolID, ticksMap)
		return
	}
	panic("StoreTicks not implemented")
}

func (m *OrderbookRepositoryMock) WithGetAllTicksFunc(ticks map[int64]orderbookdomain.OrderbookTick, ok bool) {
	m.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
		return ticks, ok
	}
}

// GetAllTicks implements OrderBookRepository.
func (m *OrderbookRepositoryMock) GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	if m.GetAllTicksFunc != nil {
		return m.GetAllTicksFunc(poolID)
	}
	panic("GetAllTicks not implemented")
}

// GetTicks implements OrderBookRepository.
func (m *OrderbookRepositoryMock) GetTicks(poolID uint64, tickIDs []int64) (map[int64]orderbookdomain.OrderbookTick, error) {
	if m.GetTicksFunc != nil {
		return m.GetTicksFunc(poolID, tickIDs)
	}
	panic("GetTicks not implemented")
}

// GetTickByID implements OrderBookRepository.
func (m *OrderbookRepositoryMock) GetTickByID(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
	if m.GetTickByIDFunc != nil {
		return m.GetTickByIDFunc(poolID, tickID)
	}
	panic("GetTickByID not implemented")
}
