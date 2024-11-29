package pipeline

import (
	"fmt"
	"sync"
)

// Iterator interface defines methods for filtering, sorting, and chunked access
type Iterator[K, V any] interface {
	Next() (V, error)     // Retrieves the next element and a bool indicating if it's valid
	HasNext() bool        // Checks if there are more elements
	SetOffset(offset int) // Sets the offset for starting point of iteration
	Reset()               // Resets the iterator to the start
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
// Error is returned when given key is not found, type assertion for value fails, or there are no more elements to iterate
func (it *SyncMapIterator[K, V]) Next() (V, error) {
	if it.HasNext() {
		key := it.keys[it.index]
		it.index++
		mp, ok := it.data.Load(key)
		if !ok {
			return *new(V), fmt.Errorf("key %v not found", key)
		}

		value, ok := mp.(V)
		if !ok {
			return *new(V), fmt.Errorf("invalid type assertion for key %v", key)
		}

		return value, nil
	}

	return *new(V), fmt.Errorf("no more elements")
}

// SetOffset sets the offset for the iterator.
// This is useful when client requests a subset of the result set
// and wants to start from a specific index.
func (it *SyncMapIterator[K, V]) SetOffset(offset int) {
	if offset < 0 {
		offset = 0
	}

	if offset > len(it.keys) {
		offset = len(it.keys)
	}

	it.index = offset
}

// HasNext checks if there are more elements in the iterator
func (it *SyncMapIterator[K, V]) HasNext() bool {
	return it.index < len(it.keys)
}

// Reset resets the iterator to the start
func (it *SyncMapIterator[K, V]) Reset() {
	it.index = 0
}
