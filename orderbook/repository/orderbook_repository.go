package orderbookrepository

import (
	"fmt"
	"maps"
	"sync"
	"sync/atomic"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

// poolTicks holds one pool's complete tick set as of a given block height. The
// map is immutable once published: StoreTicks copies the update into a new
// map and swaps the pointer in a single atomic store. Readers therefore
// always observe the tick state of one block, never a mixture of two.
type poolTicks struct {
	// height is the block the ticks were ingested from. Used to discard updates
	// that arrive out of order.
	height uint64
	ticks  map[int64]orderbookdomain.OrderbookTick
}

// poolTickState is the per-pool write path. writeMu serialises writers so that
// exactly one update is applied at a time: without it, concurrent writers would
// each clone the whole book and all but one of those clones would be discarded.
// Readers never take this lock; they read the published snapshot directly.
type poolTickState struct {
	writeMu  sync.Mutex
	snapshot atomic.Pointer[poolTicks]
}

type orderbookRepositoryImpl struct {
	tickMapByPoolIDLock sync.RWMutex
	tickMapByPoolID     map[uint64]*poolTickState
	ordersByPoolIDLock  sync.RWMutex
	ordersByPoolID      map[uint64][]orderbookdomain.Order
}

var _ orderbookdomain.OrderBookRepository = &orderbookRepositoryImpl{}

func New() *orderbookRepositoryImpl {
	return &orderbookRepositoryImpl{
		tickMapByPoolID:     map[uint64]*poolTickState{},
		tickMapByPoolIDLock: sync.RWMutex{},
		ordersByPoolID:      map[uint64][]orderbookdomain.Order{},
		ordersByPoolIDLock:  sync.RWMutex{},
	}
}

// poolState returns the per-pool tick state, creating it if absent.
func (o *orderbookRepositoryImpl) poolState(poolID uint64) *poolTickState {
	o.tickMapByPoolIDLock.RLock()
	state, ok := o.tickMapByPoolID[poolID]
	o.tickMapByPoolIDLock.RUnlock()

	if ok {
		return state
	}

	// Re-check under the write lock: a concurrent writer for the same pool may
	// have installed the state in the meantime, and all writers for a pool must
	// share one state for its write mutex to serialise them.
	o.tickMapByPoolIDLock.Lock()
	defer o.tickMapByPoolIDLock.Unlock()

	if state, ok = o.tickMapByPoolID[poolID]; ok {
		return state
	}

	state = &poolTickState{}
	o.tickMapByPoolID[poolID] = state

	return state
}

// loadPoolTicks returns the currently published tick set for a pool.
func (o *orderbookRepositoryImpl) loadPoolTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	o.tickMapByPoolIDLock.RLock()
	state, ok := o.tickMapByPoolID[poolID]
	o.tickMapByPoolIDLock.RUnlock()

	if !ok {
		return nil, false
	}

	current := state.snapshot.Load()
	if current == nil {
		return nil, false
	}

	return current.ticks, true
}

// GetAllTicks implements orderbookdomain.OrderBookRepository.
//
// The returned map is the published snapshot itself and must not be mutated by
// callers. It reflects a single block's tick state.
func (o *orderbookRepositoryImpl) GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	return o.loadPoolTicks(poolID)
}

// GetTicks implements orderbookdomain.OrderBookRepository.
func (o *orderbookRepositoryImpl) GetTicks(poolID uint64, tickIDs []int64) (map[int64]orderbookdomain.OrderbookTick, error) {
	ticks, ok := o.loadPoolTicks(poolID)
	if !ok {
		return nil, fmt.Errorf("ticks for pool %d not found", poolID)
	}

	ticksMap := make(map[int64]orderbookdomain.OrderbookTick, len(tickIDs))
	for _, tickID := range tickIDs {
		tick, ok := ticks[tickID]
		if !ok {
			return nil, fmt.Errorf("tick %d not found", tickID)
		}

		ticksMap[tickID] = tick
	}

	return ticksMap, nil
}

// GetTickByID implements orderbookdomain.OrderBookRepository.
func (o *orderbookRepositoryImpl) GetTickByID(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
	ticks, ok := o.loadPoolTicks(poolID)
	if !ok {
		return orderbookdomain.OrderbookTick{}, false
	}

	tick, ok := ticks[tickID]
	if !ok {
		return orderbookdomain.OrderbookTick{}, false
	}

	return tick, true
}

// StoreTicks implements orderbookdomain.OrderBookRepository.
//
// ticksMap is the pool's complete tick set as of height, and replaces the
// pool's existing set rather than being merged into it: the node sends every
// tick the contract reports for the pool, so a tick missing from the update is
// no longer part of the book, and an empty update means an empty book. The new
// set is published with a single atomic swap, so a concurrent reader sees
// either the previous complete state or the next one.
//
// Orderbook pools are processed asynchronously (one goroutine per pool per
// block), so updates for the same pool can finish out of order. height is the
// block the ticks were ingested for; an update older than what is already
// published is discarded, otherwise a slow block N could overwrite the tick set
// block N+1 had already published. Writes for a pool are serialised so that the
// height check and the publish are one step.
func (o *orderbookRepositoryImpl) StoreTicks(poolID uint64, height uint64, ticksMap map[int64]orderbookdomain.OrderbookTick) {
	state := o.poolState(poolID)

	state.writeMu.Lock()
	defer state.writeMu.Unlock()

	// Equal heights still apply: each update is a complete set, so the later one
	// is at least as current.
	if current := state.snapshot.Load(); current != nil && height < current.height {
		return
	}

	next := make(map[int64]orderbookdomain.OrderbookTick, len(ticksMap))
	maps.Copy(next, ticksMap)

	state.snapshot.Store(&poolTicks{height: height, ticks: next})
}

func (o *orderbookRepositoryImpl) StoreOrders(poolID uint64, orders []orderbookdomain.Order) error {
	o.ordersByPoolIDLock.Lock()
	o.ordersByPoolID[poolID] = orders
	o.ordersByPoolIDLock.Unlock()
	return nil
}

func (o *orderbookRepositoryImpl) GetOrders(poolID uint64, ownerAddress string) ([]orderbookdomain.Order, bool) {
	o.ordersByPoolIDLock.RLock()
	orders, ok := o.ordersByPoolID[poolID]
	o.ordersByPoolIDLock.RUnlock()
	if !ok {
		return nil, false
	}

	var result []orderbookdomain.Order
	for _, order := range orders {
		if order.Owner == ownerAddress {
			result = append(result, order)
		}
	}

	return result, true
}
