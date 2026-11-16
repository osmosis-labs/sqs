// Package recovery contains deferred helpers that contain panics in code that runs outside
// the HTTP recover middleware: goroutines of their own, and code that parses chain or
// upstream data, where one malformed value must not take the whole process down.
//
// recover stops a panic only when the deferred function calls it directly, so both helpers
// must be deferred themselves, not called from a deferred closure:
//
//	defer recovery.ToErrorf(&err, "processing orderbook %d", poolID)
//	defer recovery.Log(logger, "recovered panic in worker", counter, zap.Uint64("height", height))
package recovery

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/osmosis-labs/sqs/log"
)

// ToErrorf recovers a panic in the function that defers it and returns it from that function
// as an error in *err: "panic ", then format applied to args, then ": " and the panic value.
// The message is formatted only on a panic. The function's other named results keep the
// values they had when it panicked.
func ToErrorf(err *error, format string, args ...any) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("panic %s: %v", fmt.Sprintf(format, args...), r)
	}
}

// Log recovers a panic in the function that defers it, typically the body of a goroutine, and
// logs it with its stack and the given fields. It increments counter, if not nil, so that a
// recurring panic shows up in metrics as well as in the logs.
func Log(logger log.Logger, msg string, counter prometheus.Counter, fields ...zap.Field) {
	if r := recover(); r != nil {
		logger.Error(msg, append(fields, zap.Any("panic", r), zap.Stack("stack"))...)
		if counter != nil {
			counter.Inc()
		}
	}
}
