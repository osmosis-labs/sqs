package datafetchers

import (
	"fmt"
	"time"
)

// MapFetcher is an interface that defines a fetchers that is represented
// by mapped data internally with keys and values.
type MapFetcher[K comparable, V any] interface {
	Fetcher[map[K]V]

	// GetByKey returns the value for the given key.
	// Returns the value, the last fetch time, a boolean indicating if the data is stale, and an error.
	// The value is considered stale if 2x more time passed since last update.
	GetByKey(key K) (V, time.Time, bool, error)
}

type MapIntervalFetcher[K comparable, V any] struct {
	*IntervalFetcher[map[K]V]
}

var _ MapFetcher[uint64, uint64] = (*MapIntervalFetcher[uint64, uint64])(nil)

// NewMapFetcher returns a new MapIntervalFetcher.
func NewMapFetcher[K comparable, V any](updateFn func() (map[K]V, error), interval time.Duration) *MapIntervalFetcher[K, V] {
	return &MapIntervalFetcher[K, V]{
		IntervalFetcher: NewIntervalFetcher(updateFn, interval),
	}
}

// GetByKey returns the value for the given key.
func (p *MapIntervalFetcher[K, V]) GetByKey(key K) (V, time.Time, bool, error) {
	isStale := false

	// If 2x more time passed since last update, set stale flag
	timeDiff := time.Since(p.lastRetrievedTime)
	if timeDiff > 2*p.interval {
		isStale = true
	}

	resultMap, _, err := p.IntervalFetcher.Get()
	if err != nil {
		var zeroValue V
		return zeroValue, p.lastRetrievedTime, isStale, err
	}

	value, ok := resultMap[key]
	if !ok {
		var zeroValue V
		return zeroValue, p.lastRetrievedTime, isStale, fmt.Errorf("key not found: %v", key)
	}

	return value, p.lastRetrievedTime, isStale, nil
}
