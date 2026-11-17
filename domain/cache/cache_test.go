package cache_test

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain/cache"
)

// This test does basic validation against concurrency.
// That is, it tests that there are no deadlocks.
func TestConcurrentCache(t *testing.T) {
	cache := cache.New()

	seed := int64(10)

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
	for i := range numGoroutines {
		go func(index int) {
			defer wg.Done()

			// A generator is not safe for concurrent use, so each goroutine has its own.
			rand := rand.New(rand.NewSource(seed + int64(index)))

			for range numRunsPerRoutine {

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
		value       any
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

	// A key that was never set does not exist.
	if _, exists := c.Get("never-set"); exists {
		t.Errorf("Expected a key that was never set not to exist")
	}
}

// Validates that expired items are removed by the sweep in Set even if they are never read
// again, and that the sweep keeps the items that have not expired. The cache is smaller than
// one sweep batch, so a single Set sweeps all of it.
func TestCache_SweepsExpiredItems(t *testing.T) {
	c := cache.New()

	c.Set("expired", 1, time.Millisecond)
	c.Set("live", 2, time.Hour)
	c.Set("no-expiry", 3, cache.NoExpirationTTL)

	time.Sleep(10 * time.Millisecond)

	c.Set("other", 4, time.Hour)
	require.Equal(t, 3, c.Len())

	_, found := c.Get("expired")
	require.False(t, found)
	for _, key := range []string{"live", "no-expiry", "other"} {
		_, found := c.Get(key)
		require.True(t, found, key)
	}
}

// Validates that one Set examines at most one sweep batch, so that the time it holds the write
// lock does not grow with the cache.
func TestCache_SweepIsBounded(t *testing.T) {
	c := cache.New()

	const numExpired = 4 * cache.SweepBatchSize
	for i := 0; i < numExpired; i++ {
		c.Set(fmt.Sprintf("expired-%d", i), i, 50*time.Millisecond)
	}
	// The TTL is long enough that no item expires while they are inserted, so none has
	// been swept yet; wait for all of them to expire.
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, numExpired, c.Len())

	c.Set("live", 0, time.Hour)

	// Every examined item is expired, so exactly one batch is removed and one item is added.
	require.Equal(t, numExpired-cache.SweepBatchSize+1, c.Len())
}
