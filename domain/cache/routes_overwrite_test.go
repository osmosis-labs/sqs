package cache_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/domain/cache"
)

func TestRoutesOverwrite_Set(t *testing.T) {
	tests := []struct {
		name                     string
		isRoutesOverwriteEnabled bool
		key                      string
		value                    interface{}
		expectedExists           bool
		expectedValue            interface{}
	}{
		{
			name:                     "Cache Enabled - Set Value",
			isRoutesOverwriteEnabled: true,
			key:                      "key1",
			value:                    "value1",
			expectedExists:           true,
			expectedValue:            "value1",
		},
		{
			name:                     "Cache Disabled - Set Value",
			isRoutesOverwriteEnabled: false,
			key:                      "key2",
			value:                    "value2",
			expectedExists:           false,
			expectedValue:            nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := cache.CreateRoutesOverwrite(tt.isRoutesOverwriteEnabled)
			r.Set(tt.key, tt.value)

			// Additional assertions
			value, exists := r.Get(tt.key)
			if exists != tt.expectedExists {
				t.Errorf("Expected key %s to exist: %v, got: %v", tt.key, tt.expectedExists, exists)
			}

			if value != tt.expectedValue {
				t.Errorf("Expected value for key %s: %v, got: %v", tt.key, tt.expectedValue, value)
			}
		})
	}
}

func TestRoutesOverwrite_Get(t *testing.T) {
	// Assuming Set method is working correctly.
	// Setting up some initial data for testing Get method.

	r := cache.NewRoutesOverwrite()
	r.Set("key1", "value1")
	r.Set("key2", "value2")

	tests := []struct {
		name                     string
		isRoutesOverwriteEnabled bool
		key                      string
		expectedValue            interface{}
		expectedExists           bool
	}{
		{
			name:                     "Cache Enabled - Key Exists",
			isRoutesOverwriteEnabled: true,
			key:                      "key1",
			expectedValue:            "value1",
			expectedExists:           true,
		},
		{
			name:                     "Cache Enabled - Key Does Not Exist",
			isRoutesOverwriteEnabled: true,
			key:                      "key3",
			expectedValue:            nil,
			expectedExists:           false,
		},
		{
			name:                     "Cache Disabled - Key Exists",
			isRoutesOverwriteEnabled: false,
			key:                      "key2",
			expectedValue:            nil,
			expectedExists:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, exists := r.Get(tt.key)

			if value != tt.expectedValue || exists != tt.expectedExists {
				t.Errorf("Got (%v, %v), expected (%v, %v)", value, exists, tt.expectedValue, tt.expectedExists)
			}
		})
	}
}

func TestRoutesOverwrite_Delete(t *testing.T) {
	// Assuming Set method is working correctly.
	// Setting up some initial data for testing Delete method.

	r := cache.NewRoutesOverwrite()
	r.Set("key1", "value1")
	r.Set("key2", "value2")

	tests := []struct {
		name                     string
		isRoutesOverwriteEnabled bool
		key                      string
		expectedExists           bool
		expectedValue            interface{}
	}{
		{
			name:                     "Cache Enabled - Delete Key",
			isRoutesOverwriteEnabled: true,
			key:                      "key1",
			expectedExists:           false,
			expectedValue:            nil,
		},
		{
			name:                     "Cache Enabled - Delete Non-Existing Key",
			isRoutesOverwriteEnabled: true,
			key:                      "key3",
			expectedExists:           false,
			expectedValue:            nil,
		},
		{
			name:                     "Cache Disabled - Delete Key",
			isRoutesOverwriteEnabled: false,
			key:                      "key2",
			expectedExists:           false,
			expectedValue:            nil,
		},
		// Add more test cases as needed
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.Delete(tt.key)

			// Additional assertions
			value, exists := r.Get(tt.key)
			if exists != tt.expectedExists {
				t.Errorf("Expected key %s to exist: %v, got: %v", tt.key, tt.expectedExists, exists)
			}

			if value != tt.expectedValue {
				t.Errorf("Expected value for key %s: %v, got: %v", tt.key, tt.expectedValue, value)
			}
		})
	}
}
