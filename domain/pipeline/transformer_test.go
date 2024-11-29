package pipeline

import (
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
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

			require.Equal(t, tt.expected, got, "Expected count %d, but got %d", tt.expected, got)
		})
	}
}

func TestSyncMapTransformerRange(t *testing.T) {
	testCases := []struct {
		name           string
		initialData    []testdata
		expectedKeys   []string
		expectedValues []int
		stopAfter      int
	}{
		{
			name: "Empty Map",
		},
		{
			name: "Single Element",
			initialData: []testdata{
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
			initialData: []testdata{
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
			initialData: []testdata{
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
			require.Equal(t, len(tc.expectedKeys), len(collectedKeys), "Collected %d keys, want %d", len(collectedKeys), len(tc.expectedKeys))

			// Validate keys and values
			require.True(t, slices.Equal(tc.expectedKeys, collectedKeys), "Collected keys %v, want %v", collectedKeys, tc.expectedKeys)
			require.True(t, slices.Equal(tc.expectedValues, collectedValues), "Collected values %v, want %v", collectedValues, tc.expectedValues)
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
			require.Equal(t, tt.want, transformer.Data(), "Filter() modified original data. Got %v, want %v", transformer.Data(), tt.want)
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
			require.Equal(t, tt.expected, got, "Expected %v, but got %v", tt.expected, got)
		})
	}
}

func TestSyncMapTransformerKeys(t *testing.T) {
	testCases := []struct {
		name         string
		initialKeys  []string
		expectedKeys []string
	}{
		{
			name: "Empty Map",
		},
		{
			name:         "Single Element",
			initialKeys:  []string{"one"},
			expectedKeys: []string{"one"},
		},
		{
			name:         "Multiple Elements",
			initialKeys:  []string{"one", "two", "three"},
			expectedKeys: []string{"one", "two", "three"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create transformer, we are not using NewSyncMapTransformer because
			// we want to keep the keys in a specific order
			transformer := &SyncMapTransformer[string, int]{data: nil, keys: tc.initialKeys}

			collectedKeys := transformer.Keys()
			if slices.Equal(tc.expectedKeys, collectedKeys) != true {
				t.Errorf("Collected keys %v, want %v", collectedKeys, tc.expectedKeys)
			}
		})
	}
}

func TestSyncMapTransformerData(t *testing.T) {
	testCases := []struct {
		name           string
		initialData    []testdata
		expectedValues []int
		stopAfter      int
	}{
		{
			name: "Empty Map",
		},
		{
			name: "Single Element",
			initialData: []testdata{
				{
					key:   "one",
					value: 1,
				},
			},
			expectedValues: []int{1},
		},
		{
			name: "Multiple Elements",
			initialData: []testdata{
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
			expectedValues: []int{1, 2, 3},
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

			// Collect values
			collectedValues := transformer.Data()

			// Validate values
			if slices.Equal(tc.expectedValues, collectedValues) != true {
				t.Errorf("Collected values %v, want %v", collectedValues, tc.expectedValues)
			}
		})
	}
}

func TestSyncMapTransformerClone(t *testing.T) {
	testCases := []struct {
		name           string
		initialData    []testdata
		expectedValues []int
		stopAfter      int
	}{
		{
			name: "Empty Map",
		},
		{
			name: "Single Element",
			initialData: []testdata{
				{
					key:   "one",
					value: 1,
				},
			},
			expectedValues: []int{1},
		},
		{
			name: "Multiple Elements",
			initialData: []testdata{
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
			expectedValues: []int{1, 2, 3},
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

			// Clone the transformer
			clonedTransformer := transformer.Clone()

			// Verify the clone has the same underlying map
			require.Equal(t, transformer.data, clonedTransformer.data, "Clone should share the same underlying sync.Map")

			// Verify the clone has the same keys
			require.Equal(t, len(transformer.keys), len(clonedTransformer.keys), "Clone should have the same number of keys")
		})
	}
}
