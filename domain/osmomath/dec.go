package osmomath

import cosmosmath "github.com/osmosis-labs/osmosis/osmomath"

// MinBigDec returns the minimum value from a slice of osmomath.BigDec.
func MinBigDec(a ...cosmosmath.BigDec) cosmosmath.BigDec {
	if len(a) == 0 {
		return cosmosmath.BigDec{}
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
func MaxBigDec(a ...cosmosmath.BigDec) cosmosmath.BigDec {
	if len(a) == 0 {
		return cosmosmath.BigDec{}
	}
	max := a[0]
	for _, b := range a {
		if b.GT(max) {
			max = b
		}
	}
	return max
}
