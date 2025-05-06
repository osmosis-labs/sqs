package atomic

import (
	"sync"
	"testing"
)

func TestAtomicCopyOnWrite(t *testing.T) {
	tests := []struct {
		name     string
		op       func(*AtomicCopyOnWrite[int])
		expected int
	}{
		{
			name: "Store and Load single value",
			op: func(cow *AtomicCopyOnWrite[int]) {
				cow.Store(42)
			},
			expected: 42,
		},
		{
			name: "Store and Load multiple values",
			op: func(cow *AtomicCopyOnWrite[int]) {
				cow.Store(1)
				cow.Store(2)
				cow.Store(3)
			},
			expected: 3,
		},
		{
			name:     "Load without Store",
			op:       func(cow *AtomicCopyOnWrite[int]) {},
			expected: 0,
		},
		{
			name: "Concurrent Store and Load",
			op: func(cow *AtomicCopyOnWrite[int]) {
				var wg sync.WaitGroup
				for i := range 100 {
					wg.Add(2)
					go func(v int) {
						defer wg.Done()
						cow.Store(v)
					}(i)

					go func() {
						defer wg.Done()
						_ = cow.Load()
					}()
				}

				wg.Wait()
			},
			expected: -1, // final value is indeterminate due to concurrent writes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cow := NewAtomicCopyOnWrite[int]()
			tt.op(cow)
			result := cow.Load()
			if result != tt.expected && tt.expected != -1 {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestConcurrentOperations(t *testing.T) {
	cow := NewAtomicCopyOnWrite[int]()
	var wg sync.WaitGroup
	iterations := 1000

	// Concurrent writes
	for i := range iterations {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			cow.Store(v)
		}(i)
	}

	// Concurrent reads
	for range iterations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cow.Load()
		}()
	}

	wg.Wait()

	// Final value should be in the range [0, iterations-1]
	finalValue := cow.Load()
	if finalValue < 0 || finalValue >= iterations {
		t.Errorf("Unexpected final value: %d", finalValue)
	}
}
