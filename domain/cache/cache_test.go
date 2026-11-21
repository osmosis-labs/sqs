package cache_test

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain/cache"
)

// This test does basic validation against concurrency.
// That is, it tests that there are no deadlocks.
func TestConcurrentCache(t *testing.T) {
	cache := cache.New("test")

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
	c := cache.New("test")

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
	c := cache.New("test")

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
	c := cache.New("test")

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

// Validates that the cache never grows beyond its configured maxEntries, that updating an
// existing key at capacity does not evict other entries, and that eviction prefers the
// soonest-expiring entry over longer-lived and non-expiring entries.
func TestCache_RespectsMaxEntries(t *testing.T) {
	t.Run("hard cap on distinct unexpired keys", func(t *testing.T) {
		const maxEntries = 10
		c := cache.NewWithMaxEntries("test", maxEntries)

		for i := range maxEntries * 5 {
			c.Set(fmt.Sprintf("k-%d", i), i, time.Hour)
			require.LessOrEqual(t, c.Len(), maxEntries)
		}
		require.Equal(t, maxEntries, c.Len())
	})

	t.Run("updating an existing key at capacity does not evict", func(t *testing.T) {
		c := cache.NewWithMaxEntries("test", 3)
		c.Set("a", 1, time.Hour)
		c.Set("b", 2, time.Hour)
		c.Set("c", 3, time.Hour)

		c.Set("b", 20, time.Hour)
		require.Equal(t, 3, c.Len())

		for key, want := range map[string]int{"a": 1, "b": 20, "c": 3} {
			got, ok := c.Get(key)
			require.True(t, ok, key)
			require.Equal(t, want, got)
		}
	})

	t.Run("evicts soonest-expiring entry when cache fits within sample size", func(t *testing.T) {
		c := cache.NewWithMaxEntries("test", 3)
		c.Set("permanent", 1, cache.NoExpirationTTL)
		c.Set("long", 2, time.Hour)
		c.Set("short", 3, time.Minute)

		c.Set("new", 4, time.Hour)
		require.Equal(t, 3, c.Len())

		_, found := c.Get("short")
		require.False(t, found)
		for _, key := range []string{"permanent", "long", "new"} {
			_, found := c.Get(key)
			require.True(t, found, key)
		}
	})
}

// evictions returns the number of entries the cache named name evicted at capacity for reason.
func evictions(name, reason string) float64 {
	return testutil.ToFloat64(cache.EvictionsCounter.WithLabelValues(name, reason))
}

// Validates that eviction at capacity is counted under the cache's name and the evicted entry's
// reason, and that a Set below capacity or of an existing key is not.
func TestCache_CountsEvictions(t *testing.T) {
	const name = "test_counts_evictions"
	c := cache.NewWithMaxEntries(name, 2)

	c.Set("a", 1, time.Hour)
	c.Set("b", 2, time.Hour)
	c.Set("b", 3, time.Hour)
	require.Zero(t, evictions(name, cache.EvictionReasonUnexpired))

	c.Set("c", 4, time.Hour)
	require.Equal(t, float64(1), evictions(name, cache.EvictionReasonUnexpired))
	require.Zero(t, evictions(name, cache.EvictionReasonExpired))

	// An entry that expired but was not swept yet is evicted as expired.
	cache.EvictOne(c, time.Now().Add(2*time.Hour))
	require.Equal(t, float64(1), evictions(name, cache.EvictionReasonExpired))
	require.Equal(t, float64(1), evictions(name, cache.EvictionReasonUnexpired))
	require.Equal(t, 1, c.Len())

	// Another cache's evictions are counted under its own name.
	require.Zero(t, evictions("test_counts_evictions_other", cache.EvictionReasonUnexpired))
}

// Validates eviction in a cache larger than one eviction sample: the cache stays at its cap,
// every new key evicts exactly one entry, and a non-expiring entry is never chosen while the
// sample also holds expiring ones, which it always does when all but one entry expire.
func TestCache_SampledEvictionAboveSampleSize(t *testing.T) {
	const (
		name       = "test_sampled_eviction"
		maxEntries = 64 * cache.EvictSampleSize
		newKeys    = 4 * maxEntries
	)
	c := cache.NewWithMaxEntries(name, maxEntries)

	c.Set("permanent", 0, cache.NoExpirationTTL)
	for i := 1; i < maxEntries; i++ {
		c.Set(fmt.Sprintf("old-%d", i), i, time.Hour+time.Duration(i)*time.Second)
	}
	require.Equal(t, maxEntries, c.Len())

	for i := range newKeys {
		key := fmt.Sprintf("new-%d", i)
		c.Set(key, i, 2*time.Hour)
		require.Equal(t, maxEntries, c.Len())
		_, found := c.Get(key)
		require.True(t, found, key)
	}

	_, found := c.Get("permanent")
	require.True(t, found)
	require.Equal(t, float64(newKeys), evictions(name, cache.EvictionReasonUnexpired))
	require.Zero(t, evictions(name, cache.EvictionReasonExpired))
}

// Validates that a cache larger than one eviction sample whose entries never expire still
// evicts exactly one entry per new key, falling back to a sampled entry.
func TestCache_EvictsNonExpiringFallback(t *testing.T) {
	const (
		name       = "test_non_expiring_fallback"
		maxEntries = 4 * cache.EvictSampleSize
	)
	c := cache.NewWithMaxEntries(name, maxEntries)

	for i := range maxEntries {
		c.Set(fmt.Sprintf("old-%d", i), i, cache.NoExpirationTTL)
	}

	c.Set("new", -1, cache.NoExpirationTTL)
	require.Equal(t, maxEntries, c.Len())
	_, found := c.Get("new")
	require.True(t, found)

	missing := 0
	for i := range maxEntries {
		if _, found := c.Get(fmt.Sprintf("old-%d", i)); !found {
			missing++
		}
	}
	require.Equal(t, 1, missing)
	require.Equal(t, float64(1), evictions(name, cache.EvictionReasonUnexpired))
}

// Validates that concurrent Set, Get and Delete at capacity keep the cache within its cap and
// return only values that were set. Run with -race.
func TestCache_ConcurrentAtCapacity(t *testing.T) {
	const (
		maxEntries    = 2 * cache.EvictSampleSize
		keySpace      = 8 * maxEntries
		numGoroutines = 8
		opsPerRoutine = 2_000
	)
	c := cache.NewWithMaxEntries("test_concurrent_at_capacity", maxEntries)

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)
	for g := range numGoroutines {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			for range opsPerRoutine {
				key := fmt.Sprintf("k-%d", rng.Intn(keySpace))
				switch rng.Intn(3) {
				case 0:
					ttl := time.Duration(rng.Intn(5)) * time.Millisecond
					c.Set(key, key, ttl)
				case 1:
					if value, found := c.Get(key); found && value != key {
						errCh <- fmt.Errorf("key %s holds %v", key, value)
						return
					}
				default:
					c.Delete(key)
				}
				if n := c.Len(); n > maxEntries {
					errCh <- fmt.Errorf("cache holds %d entries, cap is %d", n, maxEntries)
					return
				}
			}
		}(int64(g))
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
	require.LessOrEqual(t, c.Len(), maxEntries)
}
