package pipeline

import (
	"sort"
	"sync"
)

// Transformer defines a generic interface for filtering and sorting data.
type Transformer[K, V any] interface {
	Filter(fn func(V) bool) Transformer[K, V]       // Filter applies a filter to the data
	Sort(less ...func(V, V) bool) Transformer[K, V] // Sort sorts the data
	Keys() []string                                 // Keys returns the list of transformed keys
}

// SyncMapTransformer is a generic data transformer for map data
type SyncMapTransformer[K, V any] struct {
	data *sync.Map
	keys []K
}

// NewDataTransformer initializes a transformer with raw data.
func NewSyncMapTransformer[K, V any](m *sync.Map) *SyncMapTransformer[K, V] {
	var keys []K
	m.Range(func(key, value any) bool {
		k, ok := key.(K)
		if ok {
			keys = append(keys, k)
		}
		return true // keep iterating
	})
	return &SyncMapTransformer[K, V]{data: m, keys: keys}
}

// Count returns the number of elements in the transformer.
// Number of elements returned excludes any filtered elements.
func (dt *SyncMapTransformer[K, V]) Count() uint64 {
	return uint64(len(dt.keys))
}

// Range calls f sequentially for each key and value present in the transformer.
// If f returns false, range stops the iteration.
func (dt *SyncMapTransformer[K, V]) Range(f func(key K, value V) bool) {
	for _, k := range dt.keys {
		v, ok := dt.load(k)
		if !ok {
			continue
		}

		if !f(k, v) {
			break
		}
	}
}

// Filter implements the Transformer interface for map data.
func (dt *SyncMapTransformer[K, V]) Filter(fn ...func(V) bool) *SyncMapTransformer[K, V] {
	var filteredKeys []K

	dt.Range(func(key K, value V) bool {
		for _, f := range fn {
			if f(value) {
				filteredKeys = append(filteredKeys, key)
			}
		}
		return true
	})

	dt.keys = filteredKeys

	return dt
}

// Sort implements the Transformer interface for map data.
func (dt *SyncMapTransformer[K, V]) Sort(less ...func(V, V) bool) *SyncMapTransformer[K, V] {
	if len(less) == 0 {
		return dt // no sorting required
	}

	sort.Slice(dt.keys, func(i, j int) bool {
		for _, criterion := range less {
			vi, ok := dt.load(dt.keys[i])
			if !ok {
				continue
			}

			vj, ok := dt.load(dt.keys[j])
			if !ok {
				continue
			}

			return criterion(vi, vj)
		}
		return false
	})
	return dt
}

// Keys implements the Transformer interface for map data.
func (dt *SyncMapTransformer[K, V]) Keys() []K {
	return dt.keys
}

// Data returns transformed underlying data.
func (dt *SyncMapTransformer[K, V]) Data() []V {
	var data []V
	for _, key := range dt.keys {
		v, ok := dt.load(key)
		if ok {
			data = append(data, v)
		}
	}
	return data
}

// Clone returns a new transformer with the same underlying data at the current state.
func (dt *SyncMapTransformer[K, V]) Clone() *SyncMapTransformer[K, V] {
	keys := make([]K, len(dt.keys))
	copy(keys, dt.keys)

	return &SyncMapTransformer[K, V]{
		data: dt.data,
		keys: keys,
	}
}

// load returns the value associated with the key.
// If the key is not found, it returns a zero value of the value type and false.
func (dt *SyncMapTransformer[K, V]) load(key K) (V, bool) {
	mv, ok := dt.data.Load(key)
	if !ok {
		return *new(V), false
	}

	v, ok := mv.(V)
	if !ok {
		return *new(V), false
	}

	return v, true
}
