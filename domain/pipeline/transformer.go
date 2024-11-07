package pipeline

import (
	"sort"
	"sync"
)

// Transformer defines a generic interface for filtering and sorting data.
type Transformer[K, V any] interface {
	Filter(fn func(V) bool) *Transformer[K, V]       // Filter applies a filter to the data
	Sort(less ...func(V, V) bool) *Transformer[K, V] // Sort sorts the data
	Keys() []string                                  // Keys returns the list of transformed keys
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

// Filter implements the Transformer interface for map data.
func (dt *SyncMapTransformer[K, V]) Filter(fn ...func(V) bool) *SyncMapTransformer[K, V] {
	var filteredKeys []K
	for _, key := range dt.keys {
		for _, f := range fn {
			v, ok := dt.load(key)
			if !ok {
				continue
			}

			if f(v) {
				filteredKeys = append(filteredKeys, key)
			}
		}
	}

	dt.keys = filteredKeys

	return dt
}

// Sort implements the Transformer interface for map data.
func (dt *SyncMapTransformer[K, V]) Sort(less ...func(V, V) bool) *SyncMapTransformer[K, V] {
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
