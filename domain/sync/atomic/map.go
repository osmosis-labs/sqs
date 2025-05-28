package atomic

import (
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
)

type Map[K comparable, V any] struct {
	data atomic.Value
	mu   sync.Mutex
}

func (m *Map[K, V]) Store(data map[K]V) {
	if len(data) == 0 {
		return // no data to update
	}

	m.mu.Lock() // for writers
	defer m.mu.Unlock()

	oldData, ok := m.data.Load().(map[K]V)
	if !ok {
		m.data.Store(make(map[K]V))
	}

	newData := make(map[K]V)

	maps.Copy(newData, oldData)

	for denom, value := range data {
		newData[denom] = value
	}

	m.data.Store(newData)
}

func (m *Map[K, V]) Set(id K, v V) {
	m.mu.Lock() // for writers
	defer m.mu.Unlock()

	oldData, ok := m.data.Load().(map[K]V)
	if !ok {
		m.data.Store(make(map[K]V))
	}

	newData := make(map[K]V)

	maps.Copy(newData, oldData)

	newData[id] = v

	m.data.Store(newData)
}

func (m *Map[K, V]) Load() (map[K]V, error) {
	data, ok := m.data.Load().(map[K]V)
	if !ok {
		return make(map[K]V), fmt.Errorf("failed to cast map data")
	}
	return data, nil
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	data, ok := m.data.Load().(map[K]V)
	if !ok {
		return *new(V), false
	}

	result, exists := data[key]
	if !exists {
		return *new(V), false
	}

	return result, true
}
