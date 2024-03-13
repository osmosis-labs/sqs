package usecase

import "github.com/osmosis-labs/osmosis/osmomath"

var (
	TenE9 = osmomath.NewInt(1_000_000_000)
	TenE8 = osmomath.NewInt(100_000_000)
	TenE7 = osmomath.NewInt(10_000_000)
	TenE6 = osmomath.NewInt(1_000_000)
	TenE5 = osmomath.NewInt(100_000)
	TenE4 = osmomath.NewInt(10_000)
	TenE3 = osmomath.NewInt(1_000)
	TenE2 = osmomath.NewInt(100)
	TenE1 = osmomath.NewInt(10)
)

// GetPrecomputeOrderOfMagnitude returns the order of magnitude of the given amount.
// Uses look up table for precomputed order of magnitudes.
func GetPrecomputeOrderOfMagnitude(amount osmomath.Int) int {
	if amount.GT(TenE9) {
		a := amount.Quo(TenE9)
		return 9 + GetPrecomputeOrderOfMagnitude(a)
	}
	if amount.GTE(TenE9) {
		return 9
	}
	if amount.GTE(TenE8) {
		return 8
	}
	if amount.GTE(TenE7) {
		return 7
	}
	if amount.GTE(TenE6) {
		return 6
	}
	if amount.GTE(TenE5) {
		return 5
	}
	if amount.GTE(TenE4) {
		return 4
	}
	if amount.GTE(TenE3) {
		return 3
	}
	if amount.GTE(TenE2) {
		return 2
	}
	if amount.GTE(TenE1) {
		return 1
	}

	return 0
}
