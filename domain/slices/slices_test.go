package slices_test

import (
	"reflect"
	"testing"

	"github.com/osmosis-labs/sqs/domain/slices"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		size     int
		expected [][]int
	}{
		{
			name:     "empty slice",
			input:    []int{},
			size:     3,
			expected: nil,
		},
		{
			name:     "slice smaller than chunk size",
			input:    []int{1, 2},
			size:     3,
			expected: [][]int{{1, 2}},
		},
		{
			name:     "slice equal to chunk size",
			input:    []int{1, 2, 3},
			size:     3,
			expected: [][]int{{1, 2, 3}},
		},
		{
			name:     "slice larger than chunk size",
			input:    []int{1, 2, 3, 4, 5},
			size:     2,
			expected: [][]int{{1, 2}, {3, 4}, {5}},
		},
		{
			name:     "slice multiple of chunk size",
			input:    []int{1, 2, 3, 4, 5, 6},
			size:     2,
			expected: [][]int{{1, 2}, {3, 4}, {5, 6}},
		},
		{
			name:     "chunk size of 1",
			input:    []int{1, 2, 3},
			size:     1,
			expected: [][]int{{1}, {2}, {3}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slices.Split(tt.input, tt.size)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Split(%v, %d) = %v, want %v", tt.input, tt.size, result, tt.expected)
			}
		})
	}
}
