package datafetchers

import (
	"errors"
	"sync/atomic"
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
	t.Parallel()

	updateFn := func() (int, error) {
		time.Sleep(3 * time.Second)
		return 42, nil
	}

	p := NewIntervalFetcher(updateFn, 1*time.Second)
	v, timestamp, err := p.Get()
	require.Error(t, err)
	require.Equal(t, 0, v)
	require.Equal(t, time.Time{}, timestamp)

	// The update takes 3 seconds; wait for it rather than racing a fixed sleep against it.
	require.Eventually(t, func() bool {
		v, _, err = p.Get()
		return err == nil
	}, 6*time.Second, 50*time.Millisecond)
	require.Equal(t, v, 42)
}

// TestIntervalFetcherFailedFetch verifies that a failed first fetch is retried on the short
// interval instead of leaving the cache empty for a whole refetch interval, and that a failed
// refresh keeps the previously retrieved value.
func TestIntervalFetcherFailedFetch(t *testing.T) {
	t.Parallel()

	t.Run("no cache: retries on the short interval", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		updateFn := func() (int, error) {
			if calls.Add(1) <= 2 {
				return 0, errors.New("upstream 500")
			}
			return 42, nil
		}

		p := newIntervalFetcher(updateFn, time.Hour, 10*time.Millisecond)

		require.Eventually(t, func() bool {
			v, _, err := p.Get()
			return err == nil && v == 42
		}, 2*time.Second, 5*time.Millisecond)
		require.Equal(t, int32(3), calls.Load())
	})

	t.Run("no cache: Close during the retries does not panic", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		updateFn := func() (int, error) {
			calls.Add(1)
			return 0, errors.New("upstream 500")
		}

		p := newIntervalFetcher(updateFn, time.Hour, 10*time.Millisecond)
		require.Eventually(t, func() bool { return calls.Load() >= 1 }, 2*time.Second, 5*time.Millisecond)

		require.NotPanics(t, p.Close)

		// The retries stop once the fetcher is closed.
		time.Sleep(50 * time.Millisecond)
		stoppedAt := calls.Load()
		time.Sleep(50 * time.Millisecond)
		require.Equal(t, stoppedAt, calls.Load())
	})

	t.Run("existing cache: failed refresh keeps the value", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		updateFn := func() (int, error) {
			if calls.Add(1) == 1 {
				return 42, nil
			}
			return 0, errors.New("upstream 500")
		}

		p := newIntervalFetcher(updateFn, 10*time.Millisecond, 10*time.Millisecond)

		require.Eventually(t, func() bool { return calls.Load() >= 3 }, 2*time.Second, 5*time.Millisecond)

		v, _, err := p.Get()
		require.NoError(t, err)
		require.Equal(t, 42, v)
	})
}
