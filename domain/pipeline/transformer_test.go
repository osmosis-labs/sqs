package pipeline

import (
	"reflect"
	"slices"
	"sync"
	"testing"
)

func TestSyncMapTransformer_Count(t *testing.T) {
	tests := []struct {
		name     string
		data     []int
		expected uint64
	}{
		{
			name:     "Empty map",
			data:     nil,
			expected: 0,
		},
		{
			name:     "Map with one element",
			data:     []int{1},
			expected: 1,
		},
		{
			name:     "Map with multiple elements",
			data:     []int{1, 2, 3},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m sync.Map
			for k, v := range tt.data {
				m.Store(k, v)
			}

			transformer := NewSyncMapTransformer[int, int](&m)
			got := transformer.Count()

			if got != tt.expected {
				t.Errorf("Expected count %d, but got %d", tt.expected, got)
			}
		})
	}
}

func TestSyncMapTransformerRange(t *testing.T) {
	type data struct {
		key   string
		value int
	}
	testCases := []struct {
		name           string
		initialData    []data
		expectedKeys   []string
		expectedValues []int
		stopAfter      int
	}{
		{
			name: "Empty Map",
		},
		{
			name: "Single Element",
			initialData: []data{
				{
					key:   "one",
					value: 1,
				},
			},
			expectedKeys:   []string{"one"},
			expectedValues: []int{1},
		},
		{
			name: "Multiple Elements",
			initialData: []data{
				{
					key:   "one",
					value: 1,
				},
				{
					key:   "two",
					value: 2,
				},
				{
					key:   "three",
					value: 3,
				},
			},
			expectedKeys:   []string{"one", "two", "three"},
			expectedValues: []int{1, 2, 3},
		},
		{
			name: "Stop Iteration Early",
			initialData: []data{
				{
					key:   "one",
					value: 1,
				},
				{
					key:   "two",
					value: 2,
				},
				{
					key:   "three",
					value: 3,
				},
			},
			expectedKeys:   []string{"one", "two"},
			expectedValues: []int{1, 2},
			stopAfter:      2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Construct sync.Map and populate with initial data
			m := sync.Map{}
			var keys []string

			for _, v := range tc.initialData {
				m.Store(v.key, v.value)
				keys = append(keys, v.key)
			}

			// Create transformer, we are not using NewSyncMapTransformer because
			// we want to keep the keys in a specific order
			transformer := &SyncMapTransformer[string, int]{data: &m, keys: keys}

			// Collect keys and values during Range
			var collectedKeys []string
			var collectedValues []int

			// Define iteration function
			iterFunc := func(key string, value int) bool {
				collectedKeys = append(collectedKeys, key)
				collectedValues = append(collectedValues, value)

				// Stop iteration if stopAfter is set and reached
				if tc.stopAfter > 0 && len(collectedKeys) >= tc.stopAfter {
					return false
				}
				return true
			}

			// Perform Range
			transformer.Range(iterFunc)

			// Validate collected keys
			if len(collectedKeys) != len(tc.expectedKeys) {
				t.Errorf("Collected %d keys, want %d", len(collectedKeys), len(tc.expectedKeys))
			}

			// Validate keys and values
			if slices.Equal(tc.expectedKeys, collectedKeys) != true {
				t.Errorf("Collected keys %v, want %v", collectedKeys, tc.expectedKeys)
			}

			if slices.Equal(tc.expectedValues, collectedValues) != true {
				t.Errorf("Collected values %v, want %v", collectedValues, tc.expectedValues)
			}
		})
	}
}

func TestTransformerFilter(t *testing.T) {
	tests := []struct {
		name   string
		data   []int
		filter func(int) bool
		want   []int
	}{
		{
			name: "Filter even numbers",
			data: []int{1, 2, 3, 4, 5},
			filter: func(v int) bool {
				return v%2 == 0
			},
			want: []int{2, 4},
		},
		{
			name: "Filter numbers greater than 3",
			data: []int{1, 2, 3, 4, 5},
			filter: func(v int) bool {
				return v > 3
			},
			want: []int{4, 5},
		},
		{
			name: "Filter all",
			data: []int{1, 2, 3},
			filter: func(v int) bool {
				return true
			},
			want: []int{1, 2, 3},
		},
		{
			name: "Filter none",
			data: []int{1, 2, 3},
			filter: func(v int) bool {
				return false
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m sync.Map
			for k, v := range tt.data {
				m.Store(k, v)
			}

			transformer := NewSyncMapTransformer[int, int](&m)
			transformer.Sort(func(a, b int) bool { return a < b }) // Sort the data to ensure the order is consistent
			transformer.Filter(tt.filter)

			// Check if the original data is unchanged
			if !reflect.DeepEqual(transformer.Data(), tt.want) {
				t.Errorf("Filter() modified original data. Got %v, want %v", transformer.Data(), tt.want)
			}
		})
	}
}

func TestMapTransformerSort(t *testing.T) {
	tests := []struct {
		name     string
		data     []int
		less     func(int, int) bool
		expected []int
	}{
		{
			name:     "Sort integers ascending",
			data:     []int{3, 1, 2},
			less:     func(a, b int) bool { return a < b },
			expected: []int{1, 2, 3},
		},
		{
			name:     "Sort integers descending",
			data:     []int{3, 1, 2},
			less:     func(a, b int) bool { return a > b },
			expected: []int{3, 2, 1},
		},
		{
			name:     "Sort with equal values",
			data:     []int{1, 2, 1},
			less:     func(a, b int) bool { return a < b },
			expected: []int{1, 1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m sync.Map
			for k, v := range tt.data {
				m.Store(k, v)
			}
			transformer := NewSyncMapTransformer[int, int](&m)
			transformer.Sort(tt.less)

			got := transformer.Data()
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Expected %v, but got %v", tt.expected, got)
			}
		})
	}
}
