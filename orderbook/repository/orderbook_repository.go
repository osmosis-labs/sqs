package orderbookrepository

import (
	"fmt"
	"sync"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

type orderbookRepositoryImpl struct {
	tickMapByPoolIDLock sync.RWMutex
	tickMapByPoolID     map[uint64]*sync.Map
}

var _ orderbookdomain.OrderBookRepository = &orderbookRepositoryImpl{}

func New() *orderbookRepositoryImpl {
	return &orderbookRepositoryImpl{
		tickMapByPoolID:     map[uint64]*sync.Map{},
		tickMapByPoolIDLock: sync.RWMutex{},
	}
}

// GetAllTicks implements orderbookdomain.OrderBookRepository.
func (o *orderbookRepositoryImpl) GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	o.tickMapByPoolIDLock.RLock()
	tickMap, ok := o.tickMapByPoolID[poolID]
	o.tickMapByPoolIDLock.RUnlock()

	if !ok {
		return nil, false
	}

	ticksMap := map[int64]orderbookdomain.OrderbookTick{}
	tickMap.Range(func(key, value interface{}) bool {
		tickID, ok := key.(int64)
		if !ok {
			return false
		}

		ticksMap[tickID], ok = value.(orderbookdomain.OrderbookTick)
		return ok
	})

	return ticksMap, true
}

// GetTicks implements orderbookdomain.OrderBookRepository.
func (o *orderbookRepositoryImpl) GetTicks(poolID uint64, tickIDs []int64) (map[int64]orderbookdomain.OrderbookTick, error) {
	o.tickMapByPoolIDLock.RLock()
	tickMap, ok := o.tickMapByPoolID[poolID]
	o.tickMapByPoolIDLock.RUnlock()

	if !ok {
		return nil, fmt.Errorf("ticks for pool %d not found", poolID)
	}

	ticksMap := make(map[int64]orderbookdomain.OrderbookTick, len(tickIDs))
	for _, tickID := range tickIDs {
		tick, ok := tickMap.Load(tickID)
		if !ok {
			return nil, fmt.Errorf("tick %d not found", tickID)
		}

		ticksMap[tickID], ok = tick.(orderbookdomain.OrderbookTick)
		if !ok {
			return nil, fmt.Errorf("tick %d not found", tickID)
		}
	}

	return ticksMap, nil
}

// GetTickByID implements orderbookdomain.OrderBookRepository.
func (o *orderbookRepositoryImpl) GetTickByID(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
	o.tickMapByPoolIDLock.RLock()
	tickMap, ok := o.tickMapByPoolID[poolID]
	o.tickMapByPoolIDLock.RUnlock()

	if !ok {
		return orderbookdomain.OrderbookTick{}, false
	}

	tickData, ok := tickMap.Load(tickID)
	if !ok {
		return orderbookdomain.OrderbookTick{}, false
	}

	tick, ok := tickData.(orderbookdomain.OrderbookTick)
	if !ok {
		return orderbookdomain.OrderbookTick{}, false
	}

	return tick, true
}

// StoreTicks implements orderbookdomain.OrderBookRepository.
func (o *orderbookRepositoryImpl) StoreTicks(poolID uint64, ticksMap map[int64]orderbookdomain.OrderbookTick) {
	o.tickMapByPoolIDLock.RLock()
	tickMap, ok := o.tickMapByPoolID[poolID]
	o.tickMapByPoolIDLock.RUnlock()

	if !ok {
		tickMap = &sync.Map{}
	}

	for tickID, tick := range ticksMap {
		tickMap.Store(tickID, tick)
	}

	o.tickMapByPoolIDLock.Lock()
	o.tickMapByPoolID[poolID] = tickMap
	o.tickMapByPoolIDLock.Unlock()
}
