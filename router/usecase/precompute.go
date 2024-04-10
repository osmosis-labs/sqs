package usecase

import (
	"github.com/osmosis-labs/osmosis/osmomath"
)

// OrderLookup struct holds the precomputed order of magnitudes, indexed by bit length.
// A lookupEntry means that the bit length of the amount is the key, and the value is the order of magnitude.
// if cmpValue is not nil, then if amount < cmpValue, the order of magnitude is order - 1.
type orderLookup struct {
	orderOfMagnitude int
	cmpValue         *osmomath.Int
}

var bitLenToOrderOfMagnitude = []orderLookup{}
var maxLookupPowTen = 9
var maxLookupValue osmomath.Int

// buildLookupTable
func init() {
	nextPowTen := osmomath.NewInt(10)
	ten := osmomath.NewInt(10)
	nextBitLen := nextPowTen.BigIntMut().BitLen()
	curIndex := 0
	curBitLen := 1
	bitLenToOrderOfMagnitude = append(bitLenToOrderOfMagnitude, orderLookup{orderOfMagnitude: 0, cmpValue: nil})
	for curIndex <= maxLookupPowTen {
		if curBitLen < nextBitLen {
			bitLenToOrderOfMagnitude = append(bitLenToOrderOfMagnitude, orderLookup{orderOfMagnitude: curIndex, cmpValue: nil})
		} else {
			cmpTen := nextPowTen
			nextPowTen = nextPowTen.Mul(ten)
			nextBitLen = nextPowTen.BigIntMut().BitLen()
			curIndex++
			bitLenToOrderOfMagnitude = append(bitLenToOrderOfMagnitude, orderLookup{orderOfMagnitude: curIndex, cmpValue: &cmpTen})
		}

		curBitLen++
	}

	maxLookupValue = nextPowTen.QuoRaw(100)
}

// GetPrecomputeOrderOfMagnitude returns the order of magnitude of the given amount.
// Uses look up table for precomputed order of magnitudes.
func GetPrecomputeOrderOfMagnitude(amount osmomath.Int) int {
	bitLen := amount.BigIntMut().BitLen()
	if bitLen >= len(bitLenToOrderOfMagnitude) {
		a := amount.Quo(maxLookupValue)
		return maxLookupPowTen + GetPrecomputeOrderOfMagnitude(a)
	}

	// Lookup the result based on the bit length
	val := bitLenToOrderOfMagnitude[bitLen]
	if val.cmpValue == nil {
		return val.orderOfMagnitude
	}
	if amount.LT(*val.cmpValue) {
		return val.orderOfMagnitude - 1
	}
	return val.orderOfMagnitude
}

// func init() {
// 	curPowTen := osmomath.NewInt(1)
// 	ten := osmomath.NewInt(10)
// 	orderOfMagnitudeLookup = append(orderOfMagnitudeLookup, curPowTen)
// 	for i := 1; i <= maxLookupIndex; i++ {
// 		curPowTen = curPowTen.Mul(ten)
// 		orderOfMagnitudeLookup = append(orderOfMagnitudeLookup, curPowTen)
// 	}
// 	maxLookupValue = curPowTen
// 	maxLookupValueBitLen = maxLookupValue.BigIntMut().BitLen()
// }

// GetPrecomputeOrderOfMagnitude returns the order of magnitude of the given amount.
// Uses look up table for precomputed order of magnitudes.
// func GetPrecomputeOrderOfMagnitude(amount osmomath.Int) int {
// 	if amount.BigIntMut().BitLen() >= maxLookupValueBitLen {
// 		if amount.GT(maxLookupValue) {
// 			a := amount.Quo(maxLookupValue)
// 			return maxLookupIndex + GetPrecomputeOrderOfMagnitude(a)
// 		}
// 	}
// 	low, high := 0, len(orderOfMagnitudeLookup)-1

// 	for low <= high {
// 		mid := (low + high) / 2
// 		if amount.GT(orderOfMagnitudeLookup[mid]) {
// 			low = mid + 1
// 		} else if amount.LT(orderOfMagnitudeLookup[mid]) {
// 			high = mid - 1
// 		} else {
// 			return mid
// 		}
// 	}

// 	// If not found, return 0
// 	return 0
// }

// GetPrecomputeOrderOfMagnitude returns the order of magnitude of the given amount.
// Uses look up table for precomputed order of magnitudes.
