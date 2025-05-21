package osmomath

import "github.com/osmosis-labs/osmosis/osmomath"

// MinBigDec returns the minimum value from a slice of osmomath.BigDec.
func MinBigDec(a ...osmomath.BigDec) osmomath.BigDec {
	if len(a) == 0 {
		return osmomath.BigDec{}
	}
	min := a[0]
	for _, b := range a {
		if b.LT(min) {
			min = b
		}
	}
	return min
}

// MaxBigDec returns the maximum value from a slice of osmomath.BigDec.
func MaxBigDec(a ...osmomath.BigDec) osmomath.BigDec {
	if len(a) == 0 {
		return osmomath.BigDec{}
	}
	max := a[0]
	for _, b := range a {
		if b.GT(max) {
			max = b
		}
	}
	return max
}
