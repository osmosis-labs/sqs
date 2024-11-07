package pipeline

import "sync"

// Iterator interface defines methods for filtering, sorting, and chunked access
type Iterator[K, V any] interface {
	Next() (V, bool) // Retrieves the next element and a bool indicating if it's valid
	HasNext() bool   // Checks if there are more elements
	Reset()          // Resets the iterator to the start
}

// NewMapIterator creates an iterator over map data
func NewSyncMapIterator[K, V any](data *sync.Map, keys []K) *SyncMapIterator[K, V] {
	return &SyncMapIterator[K, V]{
		data:  data,
		keys:  keys,
		index: 0,
	}
}

// SyncMapIterator is a sample iterator for a map data structure
type SyncMapIterator[K, V any] struct {
	data  *sync.Map
	keys  []K
	index int
}

// Next retrieves the next element that matches the filter (if set), advancing the index
func (it *SyncMapIterator[K, V]) Next() (V, bool) {
	if it.HasNext() {
		key := it.keys[it.index]
		it.index++
		mp, ok := it.data.Load(key)
		if !ok {
			return *new(V), false
		}

		value, ok := mp.(V)
		if !ok {
			return *new(V), false
		}

		return value, true
	}

	return *new(V), false
}

// HasNext checks if there are more elements in the iterator
func (it *SyncMapIterator[K, V]) HasNext() bool {
	return it.index < len(it.keys)
}

// Reset resets the iterator to the start
func (it *SyncMapIterator[K, V]) Reset() {
	it.index = 0
}
