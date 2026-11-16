package recovery_test

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain/recovery"
	"github.com/osmosis-labs/sqs/log"
)

func panicsAsError(shouldPanic bool) (value int, err error) {
	defer recovery.ToErrorf(&err, "doing work")

	value = 1
	if shouldPanic {
		panic("boom")
	}
	return 2, nil
}

// Validates that ToErrorf returns a panic as an error and leaves a normal return untouched.
func TestToError(t *testing.T) {
	value, err := panicsAsError(false)
	require.NoError(t, err)
	require.Equal(t, 2, value)

	value, err = panicsAsError(true)
	require.EqualError(t, err, "panic doing work: boom")
	// The other results keep the values they had when the function panicked.
	require.Equal(t, 1, value)
}

// Validates that ToErrorf keeps an error the function returned when it does not panic.
// Validates that the message is formatted from the arguments.
func TestToErrorf_FormatsMessage(t *testing.T) {
	err := func() (err error) {
		defer recovery.ToErrorf(&err, "processing orderbook %d", 7)
		panic("boom")
	}()
	require.EqualError(t, err, "panic processing orderbook 7: boom")
}

func TestToError_KeepsReturnedError(t *testing.T) {
	returned := errors.New("returned")
	err := func() (err error) {
		defer recovery.ToErrorf(&err, "doing work")
		return returned
	}()
	require.ErrorIs(t, err, returned)
}

// Validates that Log contains a panic and counts it, and counts nothing without one.
func TestLog(t *testing.T) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_recovered_panics_total"})

	func() {
		defer recovery.Log(&log.NoOpLogger{}, "recovered panic", counter)
	}()
	require.Equal(t, float64(0), testutil.ToFloat64(counter))

	func() {
		defer recovery.Log(&log.NoOpLogger{}, "recovered panic", counter)
		panic("boom")
	}()
	require.Equal(t, float64(1), testutil.ToFloat64(counter))

	// A nil counter is allowed.
	func() {
		defer recovery.Log(&log.NoOpLogger{}, "recovered panic", nil)
		panic("boom")
	}()
}
