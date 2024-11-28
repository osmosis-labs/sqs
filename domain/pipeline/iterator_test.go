package pipeline

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

type testdata struct {
	key   string
	value int
}

// MockIterator is a simple implementation of Iterator for testing
type MockIterator struct {
	items []int
	index int
}

func (m *MockIterator) HasNext() bool {
	return m.index < len(m.items)
}

func (m *MockIterator) SetOffset(offset int) {
	m.index = offset
}

func (m *MockIterator) Next() (int, error) {
	if m.HasNext() {
		item := m.items[m.index]
		m.index++
		return item, nil
	}
	return 0, fmt.Errorf("no more elements")
}

func (m *MockIterator) Reset() {
	m.index = 0
}

func TestSyncMapIteratorNext(t *testing.T) {
	tests := []struct {
		name     string
		data     []testdata
		keys     []string
		expected []testdata
	}{
		{
			name: "Empty map",
		},
		{
			name:     "Single element",
			data:     []testdata{{key: "a", value: 1}},
			expected: []testdata{{key: "a", value: 1}},
		},
		{
			name:     "Multiple elements",
			data:     []testdata{{key: "a", value: 1}, {key: "b", value: 2}, {key: "c", value: 3}},
			expected: []testdata{{key: "a", value: 1}, {key: "b", value: 2}, {key: "c", value: 3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sync.Map{}
			var keys []string

			for _, v := range tt.data {
				m.Store(v.key, v)
				keys = append(keys, v.key)
			}

			it := NewSyncMapIterator[string, testdata](&m, keys)

			var result []testdata
			for {
				val, err := it.Next()
				if err != nil {
					break
				}
				result = append(result, val)
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Iteration result = %v, want %v", result, tt.expected)
			}

			// Test that after full iteration, Next() returns false
			_, err := it.Next()
			if err == nil {
				t.Errorf("Expected Next() to return false after full iteration")
			}
		})
	}
}

func TestSyncMapIteratorSetOffset(t *testing.T) {
	tests := []struct {
		name     string
		data     []testdata
		keys     []string
		offset   int
		expected []testdata
	}{
		{
			name:     "Set offset to 0",
			data:     []testdata{{key: "a", value: 1}, {key: "b", value: 2}, {key: "c", value: 3}},
			keys:     []string{"a", "b", "c"},
			offset:   0,
			expected: []testdata{{key: "a", value: 1}, {key: "b", value: 2}, {key: "c", value: 3}},
		},
		{
			name:     "Set offset to middle",
			data:     []testdata{{key: "a", value: 1}, {key: "b", value: 2}, {key: "c", value: 3}},
			keys:     []string{"a", "b", "c"},
			offset:   1,
			expected: []testdata{{key: "b", value: 2}, {key: "c", value: 3}},
		},
		{
			name:     "Set offset to last element",
			data:     []testdata{{key: "a", value: 1}, {key: "b", value: 2}, {key: "c", value: 3}},
			keys:     []string{"a", "b", "c"},
			offset:   2,
			expected: []testdata{{key: "c", value: 3}},
		},
		{
			name:   "Set offset beyond last element",
			data:   []testdata{{key: "a", value: 1}, {key: "b", value: 2}, {key: "c", value: 3}},
			keys:   []string{"a", "b", "c"},
			offset: 3,
		},
		{
			name:   "Set offset for empty map",
			offset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := sync.Map{}
			for _, v := range tt.data {
				m.Store(v.key, v)
			}

			it := NewSyncMapIterator[string, testdata](&m, tt.keys)
			it.SetOffset(tt.offset)

			var result []testdata
			for {
				val, err := it.Next()
				if err != nil {
					break
				}
				result = append(result, val)
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Iteration result after SetOffset(%d) = %v, want %v", tt.offset, result, tt.expected)
			}

			// Test that after full iteration, Next() returns false
			_, err := it.Next()
			if err == nil {
				t.Errorf("Expected Next() to return err after full iteration")
			}
		})
	}
}

func TestSyncMapIterator_HasNext(t *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		index int
		want  bool
	}{
		{
			name:  "Empty iterator",
			keys:  []string{},
			index: 0,
			want:  false,
		},
		{
			name:  "Iterator with elements, at start",
			keys:  []string{"a", "b", "c"},
			index: 0,
			want:  true,
		},
		{
			name:  "Iterator with elements, in middle",
			keys:  []string{"a", "b", "c"},
			index: 1,
			want:  true,
		},
		{
			name:  "Iterator with elements, at last element",
			keys:  []string{"a", "b", "c"},
			index: 2,
			want:  true,
		},
		{
			name:  "Iterator with elements, past last element",
			keys:  []string{"a", "b", "c"},
			index: 3,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iterator := &SyncMapIterator[string, int]{
				keys:  tt.keys,
				index: tt.index,
			}
			if got := iterator.HasNext(); got != tt.want {
				t.Errorf("SyncMapIterator.HasNext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncMapIterator_Reset(t *testing.T) {
	tests := []struct {
		name            string
		initialIndex    int
		keys            []string
		expectedIndex   int
		expectedHasNext bool
	}{
		{
			name:            "Reset from middle",
			initialIndex:    2,
			keys:            []string{"a", "b", "c", "d"},
			expectedIndex:   0,
			expectedHasNext: true,
		},
		{
			name:            "Reset from end",
			initialIndex:    4,
			keys:            []string{"a", "b", "c", "d"},
			expectedIndex:   0,
			expectedHasNext: true,
		},
		{
			name:            "Reset from start",
			initialIndex:    0,
			keys:            []string{"a", "b", "c", "d"},
			expectedIndex:   0,
			expectedHasNext: true,
		},
		{
			name:            "Reset empty iterator",
			initialIndex:    0,
			keys:            []string{},
			expectedIndex:   0,
			expectedHasNext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := &SyncMapIterator[string, int]{
				data:  &sync.Map{},
				keys:  tt.keys,
				index: tt.initialIndex,
			}

			it.Reset()

			if it.index != tt.expectedIndex {
				t.Errorf("After Reset(), index = %v, want %v", it.index, tt.expectedIndex)
			}

			if it.HasNext() != tt.expectedHasNext {
				t.Errorf("After Reset(), HasNext() = %v, want %v", it.HasNext(), tt.expectedHasNext)
			}
		})
	}
}
