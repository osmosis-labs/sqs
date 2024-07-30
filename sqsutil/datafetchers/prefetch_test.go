package datafetchers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWaitUntilFirstResult tests the behavior of the WaitUntilFirstResult method in the Prefetcher type.
// It verifies that the WaitUntilFirstResult method blocks until the first result is available,
// and that it returns the correct value and timestamp after the result is available.
// The test also checks that the WaitUntilFirstResult method blocks for the expected duration,
// and does not block for too long.
func TestWaitUntilFirstResult(t *testing.T) {
	updateFn := func() int {
		time.Sleep(2 * time.Second)
		return 42
	}

	start := time.Now()
	p := NewIntervalFetcher(updateFn, 1*time.Second)
	v, timestamp, err := p.Get()
	require.Error(t, err)
	require.Equal(t, 0, v)
	require.Equal(t, time.Time{}, timestamp)

	p.WaitUntilFirstResult()
	waitTimeFinished := time.Now()
	elapsedSinceStart := waitTimeFinished.Sub(start)
	requireTimeDurationInRange(t, elapsedSinceStart, 2*time.Second, 3*time.Second)

	v, timestamp, err = p.Get()
	require.NoError(t, err)
	require.Equal(t, v, 42)
	elapsedSinceStart = time.Since(start)
	requireTimeDurationInRange(t, elapsedSinceStart, 2*time.Second, 3*time.Second)
	getVsWaitTime := timestamp.Sub(waitTimeFinished)
	requireTimeDurationInRange(t, getVsWaitTime, -50*time.Millisecond, 50*time.Millisecond)
}

func requireTimeDurationInRange(t *testing.T, d time.Duration, min time.Duration, max time.Duration) {
	require.True(t, d >= min, "Duration %s is less than min %s", d, min)
	require.True(t, d <= max, "Duration %s is greater than max %s", d, max)
}
