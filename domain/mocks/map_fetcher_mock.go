package mocks

import (
	"time"

	"github.com/osmosis-labs/sqs/sqsutil/datafetchers"
)

type MapFetcherMock[K comparable, V any] struct {
	GetByKeyFn func(key K) (V, time.Time, bool, error)
}

// Get implements datafetchers.MapFetcher.
func (m *MapFetcherMock[K, V]) Get() (map[K]V, time.Time, error) {
	panic("unimplemented")
}

// GetByKey implements datafetchers.MapFetcher.
func (m *MapFetcherMock[K, V]) GetByKey(key K) (V, time.Time, bool, error) {
	if m.GetByKeyFn == nil {
		panic("GetByKeyFn is not set")
	}

	return m.GetByKeyFn(key)
}

// GetRefetchInterval implements datafetchers.MapFetcher.
func (m *MapFetcherMock[K, V]) GetRefetchInterval() time.Duration {
	panic("unimplemented")
}

// WaitUntilFirstResult implements datafetchers.MapFetcher.
func (m *MapFetcherMock[K, V]) WaitUntilFirstResult() {
	panic("unimplemented")
}

var _ datafetchers.MapFetcher[uint64, uint64] = (*MapFetcherMock[uint64, uint64])(nil)
