package atomic

import (
	"encoding/json/v2"
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

	for range goroutines {
		wg.Go(func() {
			for range iterations {
				counter.Add(1)
			}
		})
	}

	wg.Wait()

	expected := uint64(goroutines * iterations)
	if counter.Load() != expected {
		t.Errorf("Concurrent increments: got %d, want %d", counter.Load(), expected)
	}
}

func TestUint64JSON(t *testing.T) {
	type wrapper struct {
		Value *Uint64 `json:"value"`
	}

	for _, tt := range []struct {
		name  string
		input uint64
		want  string
	}{
		{name: "zero is encoded, not omitted", input: 0, want: `{"value":0}`},
		{name: "max uint64 keeps precision", input: math.MaxUint64, want: `{"value":18446744073709551615}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bz, err := json.Marshal(wrapper{Value: NewUint64(tt.input)})
			if err != nil {
				t.Fatal(err)
			}
			if string(bz) != tt.want {
				t.Fatalf("Marshal = %s; want %s", bz, tt.want)
			}

			var decoded wrapper
			if err := json.Unmarshal(bz, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Value.Load() != tt.input {
				t.Fatalf("round trip = %d; want %d", decoded.Value.Load(), tt.input)
			}
		})
	}

	for _, input := range []string{`-1`, `1.5`, `"1"`, `18446744073709551616`} {
		var decoded Uint64
		if err := json.Unmarshal([]byte(input), &decoded); err == nil {
			t.Errorf("Unmarshal(%s) succeeded; want error", input)
		}
	}
}
