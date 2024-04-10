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

// buildLookupTable initializes the bitLenToOrderOfMagnitude lookup table and sets up precomputed values for order of magnitudes.
// It iterates through bit lengths and determines the appropriate order of magnitude for each bit length.
// If a new power of ten is encountered at a particular magnitude, it includes the cmpValue alongside the order of magnitude for that power of ten.
func init() {
	// Initialize variables for building the lookup table
	nextPowTen := osmomath.NewInt(10)
	ten := osmomath.NewInt(10)
	nextBitLen := nextPowTen.BigIntMut().BitLen()
	curIndex := 0
	curBitLen := 1

	// Add the initial entry for bit length 0
	bitLenToOrderOfMagnitude = append(bitLenToOrderOfMagnitude, orderLookup{orderOfMagnitude: 0, cmpValue: nil})

	// Iterate through bit lengths and populate the lookup table
	for curIndex <= maxLookupPowTen {
		// If the current bit length is less than the bit length of the next power of ten, add an entry with no cmpValue
		if curBitLen < nextBitLen {
			bitLenToOrderOfMagnitude = append(bitLenToOrderOfMagnitude, orderLookup{orderOfMagnitude: curIndex, cmpValue: nil})
		} else {
			// If the current bit length is equal to or greater than the bit length of the next power of ten,
			// set cmpValue to the next power of ten and update variables for the next iteration
			cmpTen := nextPowTen
			nextPowTen = nextPowTen.Mul(ten)
			nextBitLen = nextPowTen.BigIntMut().BitLen()
			curIndex++
			bitLenToOrderOfMagnitude = append(bitLenToOrderOfMagnitude, orderLookup{orderOfMagnitude: curIndex, cmpValue: &cmpTen})
		}

		// Increment the current bit length
		curBitLen++
	}

	// Set maxLookupValue to 100 times the next power of ten after maxLookupPowTen
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
	// If the cmpValue is not nil, then compare the amount with cmpValue.
	val := bitLenToOrderOfMagnitude[bitLen]
	if val.cmpValue == nil {
		return val.orderOfMagnitude
	}
	if amount.LT(*val.cmpValue) {
		return val.orderOfMagnitude - 1
	}
	return val.orderOfMagnitude
}
