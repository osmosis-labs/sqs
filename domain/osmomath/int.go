package osmomath

import "github.com/osmosis-labs/osmosis/osmomath"

func SafeUint64(i osmomath.Int) uint64 {
	if i.IsNil() || i.IsZero() || i.IsNegative() {
		return 0
	}

	if !i.IsUint64() {
		return ^uint64(0)
	}

	return i.Uint64()
}
