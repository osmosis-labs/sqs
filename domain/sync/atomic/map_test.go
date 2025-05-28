package atomic

import (
	"reflect"
	"testing"
)

func TestMap_Set(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]int
		input    map[string]int
		expected map[string]int
	}{
		{
			name:     "Set new values",
			initial:  map[string]int{},
			input:    map[string]int{"a": 1, "b": 2},
			expected: map[string]int{"a": 1, "b": 2},
		},
		{
			name:     "Update existing values",
			initial:  map[string]int{"a": 1, "b": 2},
			input:    map[string]int{"b": 3, "c": 4},
			expected: map[string]int{"a": 1, "b": 3, "c": 4},
		},
		{
			name:     "Set empty map",
			initial:  map[string]int{"a": 1},
			input:    map[string]int{},
			expected: map[string]int{"a": 1},
		},
		{
			name:     "Set nil map",
			initial:  map[string]int{"a": 1},
			input:    nil,
			expected: map[string]int{"a": 1},
		},
		{
			name:     "Set on empty initial map",
			initial:  nil,
			input:    map[string]int{"a": 1},
			expected: map[string]int{"a": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Map[string, int]{}
			if tt.initial != nil {
				for key, value := range tt.initial {
					m.Set(key, value)
				}
			}

			for key, value := range tt.input {
				m.Set(key, value)
			}

			result, err := m.Load()
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMap_Get(t *testing.T) {
	tests := []struct {
		name     string
		initial  map[string]int
		key      string
		expected int
		exists   bool
	}{
		{
			name:     "Get existing value",
			initial:  map[string]int{"a": 1, "b": 2},
			key:      "a",
			expected: 1,
			exists:   true,
		},
		{
			name:     "Get non-existent value",
			initial:  map[string]int{"a": 1, "b": 2},
			key:      "c",
			expected: 0,
			exists:   false,
		},
		{
			name:     "Get from empty map",
			initial:  map[string]int{},
			key:      "a",
			expected: 0,
			exists:   false,
		},
		{
			name:     "Get zero value",
			initial:  map[string]int{"a": 0},
			key:      "a",
			expected: 0,
			exists:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Map[string, int]{}

			for key, value := range tt.initial {
				m.Set(key, value)
			}

			result, exists := m.Get(tt.key)

			if exists != tt.exists {
				t.Errorf("Expected exists = %v, got %v", tt.exists, exists)
			}

			if result != tt.expected {
				t.Errorf("Expected value %v, got %v", tt.expected, result)
			}
		})
	}
}
