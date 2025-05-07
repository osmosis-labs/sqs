package atomic

import (
	"math"
	"sync"
	"testing"
)

func TestNewUint64(t *testing.T) {
	tests := []struct {
		name     string
		input    uint64
		expected uint64
	}{
		{
			name:     "Zero value",
			input:    0,
			expected: 0,
		},
		{
			name:     "Positive value",
			input:    42,
			expected: 42,
		},
		{
			name:     "Large value",
			input:    math.MaxUint64,
			expected: math.MaxUint64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewUint64(tt.input)
			if result.Load() != tt.expected {
				t.Errorf("NewUint64(%d) = %d; want %d", tt.input, result.Load(), tt.expected)
			}
		})
	}
}

func TestNewUint64Concurrent(t *testing.T) {
	const (
		goroutines = 100
		iterations = 1000
	)

	var wg sync.WaitGroup
	counter := NewUint64(0)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				counter.Add(1)
			}
		}()
	}

	wg.Wait()

	expected := uint64(goroutines * iterations)
	if counter.Load() != expected {
		t.Errorf("Concurrent increments: got %d, want %d", counter.Load(), expected)
	}
}
