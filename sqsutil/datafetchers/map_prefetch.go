package datafetchers

import (
	"errors"
	"fmt"
	"time"

	"github.com/osmosis-labs/sqs/log"
)

// ErrKeyNotFound is returned by GetByKey when the fetched data has no value for the key.
// The fetch itself succeeded, so callers can tell a missing key apart from a failed fetch.
var ErrKeyNotFound = errors.New("key not found")

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
func NewMapFetcher[K comparable, V any](updateFn func() (map[K]V, error), interval time.Duration, logger log.Logger) *MapIntervalFetcher[K, V] {
	return &MapIntervalFetcher[K, V]{
		IntervalFetcher: NewIntervalFetcher(updateFn, interval, logger),
	}
}

// GetByKey returns the value for the given key.
func (p *MapIntervalFetcher[K, V]) GetByKey(key K) (V, time.Time, bool, error) {
	// The map and its retrieval time come from one read, so that a refetch cannot pair one
	// with the other of a different fetch. Get returns the zero time on error, which is stale.
	resultMap, lastRetrievedTime, err := p.IntervalFetcher.Get()

	// If 2x more time passed since last update, set stale flag
	isStale := time.Since(lastRetrievedTime) > 2*p.interval

	if err != nil {
		var zeroValue V
		return zeroValue, lastRetrievedTime, isStale, err
	}

	value, ok := resultMap[key]
	if !ok {
		var zeroValue V
		return zeroValue, lastRetrievedTime, isStale, fmt.Errorf("%w: %v", ErrKeyNotFound, key)
	}

	return value, lastRetrievedTime, isStale, nil
}
