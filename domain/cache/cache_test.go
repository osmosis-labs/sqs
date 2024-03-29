package cache_test

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/osmosis-labs/sqs/domain/cache"
)

// This is a basic ChatGPT generates test case for the cache.
func TestCache(t *testing.T) {
	cache := cache.New()

	const defaultKeyToSet = "key1"

	// Test cases
	testCases := []struct {
		name       string
		key        string
		value      interface{}
		expiration time.Duration
		sleep      time.Duration
		expected   interface{}
	}{
		{"ValidKey", defaultKeyToSet, "value1", time.Second * 5, 0, "value1"},
		{"ExpiredKey", defaultKeyToSet, "value2", time.Nanosecond, time.Millisecond * 10, nil},
		{"NonExistentKey", "key2", "value3", time.Second * 5, 0, nil},
	}

	// Run tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cache.Set(defaultKeyToSet, tc.value, tc.expiration)

			// Sleep if necessary to simulate expiration
			time.Sleep(tc.sleep)

			val, exists := cache.Get(tc.key)

			// Check existence and value
			if exists != (tc.expected != nil) {
				t.Errorf("Expected existence: %v, Got: %v", tc.expected != nil, exists)
			}

			// Check value if it exists
			if exists && val != tc.expected {
				t.Errorf("Expected value: %v, Got: %v", tc.expected, val)
			}
		})
	}
}

// This test does basic validation against concurrency.
// That is, it tests that there are no deadlocks.
func TestConcurrentCache(t *testing.T) {
	cache := cache.New()

	seed := int64(10)
	rand := rand.New(rand.NewSource(seed))

	// Number of goroutines
	numGoroutines := 10
	numRunsPerRoutine := 15
	maxKeyNumRand := 10
	expirationMaxMs := 100

	// Wait group to wait for all goroutines to finish
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Channel for goroutines to communicate errors
	errCh := make(chan error, numGoroutines*numRunsPerRoutine)

	// Run goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()

			for i := 0; i < numRunsPerRoutine; i++ {

				randKey := rand.Intn(maxKeyNumRand)

				// Random key and value
				key := fmt.Sprintf("key%d", randKey)
				value := "does not matter"

				// Random expiration time
				expiration := time.Millisecond * time.Duration(rand.Intn(expirationMaxMs))

				// Set value in cache
				cache.Set(key, value, expiration)

				// Simulate some random work in the goroutine
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(expirationMaxMs)))

				// Retrieve value from the cache
				val, exists := cache.Get(key)

				// Check if the retrieved value matches the expected value
				if exists && val != value {
					errCh <- fmt.Errorf("Goroutine %d: Expected value %s, Got %s", index, value, val)
				}
			}
		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Close the error channel to signal the end of errors
	close(errCh)

	// Collect errors from goroutines
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	// Check for errors
	if len(errors) > 0 {
		t.Errorf("Concurrent Cache Test failed with %d errors:", len(errors))
		for _, err := range errors {
			t.Error(err)
		}
	}
}

func TestCache_SetExpiration(t *testing.T) {
	c := cache.New()

	tests := []struct {
		name        string
		key         string
		value       interface{}
		expiration  time.Duration
		expectExist bool
	}{
		{
			name:        "Set with Expiration - Key Exists",
			key:         "key1",
			value:       "value1",
			expiration:  100 * time.Millisecond,
			expectExist: true,
		},
		{
			name:        "Set with Expiration - Key Expires",
			key:         "key2",
			value:       "value2",
			expiration:  50 * time.Millisecond,
			expectExist: false,
		},
		{
			name:        "Set with No Expiration - Key Exists",
			key:         "key3",
			value:       "value3",
			expiration:  cache.NoExpiration,
			expectExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c.Set(tt.key, tt.value, tt.expiration)
			time.Sleep(75 * time.Millisecond) // Sleep to wait for expiration in the second test case

			// Check if the key exists in the cache
			value, exists := c.Get(tt.key)
			if exists != tt.expectExist {
				t.Errorf("Expected key %s to exist: %v, got: %v", tt.key, tt.expectExist, exists)
			}

			// If the key is expected to exist, also check if the value matches
			if tt.expectExist && value != tt.value {
				t.Errorf("Expected value for key %s: %v, got: %v", tt.key, tt.value, value)
			}
		})
	}
}
