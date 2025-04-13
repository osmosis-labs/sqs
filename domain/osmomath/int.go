package osmomath

import "github.com/osmosis-labs/osmosis/osmomath"

// SafeUint64 converts an osmomath.Int to a uint64.
// It returns 0 if the input is nil, zero, or negative.
// If the input is greater than uint64 max value, it returns max uint64 value.
func SafeUint64(i osmomath.Int) uint64 {
	if i.IsNil() || i.IsZero() || i.IsNegative() {
		return 0
	}

	if !i.IsUint64() {
		return ^uint64(0)
	}

	return i.Uint64()
}
