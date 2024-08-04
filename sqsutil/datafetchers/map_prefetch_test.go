package datafetchers_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/osmosis-labs/sqs/sqsutil/datafetchers"
	"github.com/stretchr/testify/assert"
)

func TestMapIntervalFetcher_GetByKey(t *testing.T) {
	// Run the test in parallel with other tests
	t.Parallel()

	didFetchOnce := atomic.Bool{}

	// Define the update function
	updateFn := func() map[int]string {
		if didFetchOnce.Load() {
			// Intentionally block the update function to simulate a slow update
			time.Sleep(10 * time.Second)
		}

		didFetchOnce.Store(true)

		return map[int]string{
			1: "one",
			2: "two",
			3: "three",
		}
	}

	// Create a new MapIntervalFetcher with a short interval
	interval := 50 * time.Millisecond
	fetcher := datafetchers.NewMapFetcher(updateFn, interval)

	// Wait until the first result is fetched
	fetcher.WaitUntilFirstResult()

	// Test getting a valid key
	value, lastFetch, isStale, err := fetcher.GetByKey(1)
	assert.NoError(t, err)
	assert.Equal(t, "one", value)
	assert.False(t, isStale)
	assert.NotZero(t, lastFetch)

	// Test getting an invalid key
	value, lastFetch, isStale, err = fetcher.GetByKey(99)
	assert.Error(t, err)
	assert.Equal(t, "", value)
	assert.False(t, isStale)
	assert.NotZero(t, lastFetch)

	// Wait for more than 2x the interval to ensure data becomes stale
	time.Sleep(200 * time.Millisecond)

	// Test getting a key after data should be stale
	value, lastFetch, isStale, err = fetcher.GetByKey(2)
	assert.NoError(t, err)
	assert.Equal(t, "two", value)
	assert.True(t, isStale)
	assert.NotZero(t, lastFetch)
}
