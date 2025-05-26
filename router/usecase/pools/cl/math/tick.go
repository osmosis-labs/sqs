package math

import (
	"math"

	clmath "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/math"
	"github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/types"
)

var MaxSpotPrice = types.MaxSpotPrice.MustFloat64()

var oneBigFloat = 1.0

var (
	MinSpotPriceV2     = types.MinSpotPriceV2.MustFloat64()
	MaxSpotPriceBigDec = types.MaxSpotPriceBigDec.MustFloat64()
)

// TickToSqrtPrice returns the sqrtPrice given a tickIndex
// If tickIndex is zero, the function returns osmomath.OneDec().
// It is the combination of calling TickToPrice followed by Sqrt.
func TickToSqrtPrice(tickIndex int64) (float64, error) {
	priceBigFloat, err := TickToPrice(tickIndex)
	if err != nil {
		return 0, err
	}

	sqrtPrice := math.Sqrt(priceBigFloat)

	return sqrtPrice, nil
}

// TickToPrice returns the price given a tickIndex
// If tickIndex is zero, the function returns osmomath.OneDec().
func TickToPrice(tickIndex int64) (float64, error) {
	if tickIndex == 0 {
		return oneBigFloat, nil
	}

	// N.B. We special case MinInitializedTickV2 and MinCurrentTickV2 since MinInitializedTickV2
	// is the first one that requires taking 10 to the exponent of (-31 + -6) = -37
	// Given BigDec's precision of 36, that cannot be supported.
	// The fact that MinInitializedTickV2 and MinCurrentTickV2 translate to the same
	// price is acceptable since MinCurrentTickV2 cannot be initialized.
	if tickIndex == types.MinInitializedTickV2 || tickIndex == types.MinCurrentTickV2 {
		return MinSpotPriceV2, nil
	}

	numAdditiveTicks, geometricExponentDelta, err := clmath.TickToAdditiveGeometricIndices(tickIndex)
	if err != nil {
		return 0, err
	}

	// price = 10^geometricExponentDelta + numAdditiveTicks * 10^exponentAtCurrentTick
	// exponent at current tick = types.ExponentAtPriceOne + geometricExponentDelta + conditional
	// where conditional = -1 if tickIndex < 0, 0 otherwise
	// so we compute the price as (10**(geometricExponentDelta - exponentAtCurrentTick) + numAdditiveTicks) * 10**exponentAtCurrentTick
	// notice that geometricExponentDelta - exponentAtCurrentTick is either 6 or 7
	// so we compute this as unscaledPrice = (10**(geometricExponentDelta - exponentAtCurrentTick) + numAdditiveTicks)

	// Calculate the exponentAtCurrentTick from the starting exponentAtPriceOne and the geometricExponentDelta
	exponentAtCurrentTick := types.ExponentAtPriceOne + geometricExponentDelta
	var unscaledPrice int64 = 1_000_000
	if tickIndex < 0 {
		// We must decrement the exponentAtCurrentTick when entering the negative tick range in order to constantly step up in precision when going further down in ticks
		// Otherwise, from tick 0 to tick -(geometricExponentIncrementDistanceInTicks), we would use the same exponent as the exponentAtPriceOne
		exponentAtCurrentTick = exponentAtCurrentTick - 1
		unscaledPrice *= 10
	}
	unscaledPrice += numAdditiveTicks
	pw := powTenBigDec(exponentAtCurrentTick)
	price := pw * float64(unscaledPrice)

	// defense in depth, this logic would not be reached due to use having checked if given tick is in between
	// min tick and max tick.
	if price > MaxSpotPriceBigDec || price < MinSpotPriceV2 {
		return 0, types.PriceBoundError{} // TODO
		// return float64{}, cltypes.PriceBoundError{ProvidedPrice: price, MinSpotPrice: cltypes.MinSpotPriceV2, MaxSpotPrice: cltypes.MaxSpotPrice}
	}
	return price, nil
}

func TickToAdditiveGeometricIndices(tickIndex int64) (additiveTicks int64, geometricExponentDelta int64, err error) {
	if tickIndex == 0 {
		return 0, 0, nil
	}

	// N.B. We special case MinInitializedTickV2 and MinCurrentTickV2 since MinInitializedTickV2
	// is the first one that requires taking 10 to the exponent of (-31 + -6) = -37
	// Given BigDec's precision of 36, that cannot be supported.
	// The fact that MinInitializedTickV2 and MinCurrentTickV2 translate to the same
	// price is acceptable since MinCurrentTickV2 cannot be initialized.
	if tickIndex == types.MinInitializedTickV2 || tickIndex == types.MinCurrentTickV2 {
		return 0, -30, nil
	}

	// Check that the tick index is between min and max value
	if tickIndex < types.MinCurrentTickV2 {
		return 0, 0, types.TickIndexMinimumError{MinTick: types.MinCurrentTickV2}
	}
	if tickIndex > types.MaxTick {
		return 0, 0, types.TickIndexMaximumError{MaxTick: types.MaxTick}
	}

	// Use floor division to determine what the geometricExponent is now (the delta from the starting exponentAtPriceOne)
	geometricExponentDelta = tickIndex / geometricExponentIncrementDistanceInTicks

	// Now, starting at the minimum tick of the current increment, we calculate how many ticks in the current geometricExponent we have passed
	numAdditiveTicks := tickIndex - (geometricExponentDelta * geometricExponentIncrementDistanceInTicks)
	return numAdditiveTicks, geometricExponentDelta, nil
}

// RoundDownTickToSpacing rounds the tick index down to the nearest tick spacing if the tickIndex is in between authorized tick values
// Note that this is Euclidean modulus.
// The difference from default Go modulus is that Go default results
// in a negative remainder when the dividend is negative.
// Consider example tickIndex = -17, tickSpacing = 10
// tickIndexModulus = tickIndex % tickSpacing = -7
// tickIndexModulus = -7 + 10 = 3
// tickIndex = -17 - 3 = -20
func RoundDownTickToSpacing(tickIndex int64, tickSpacing int64) (int64, error) {
	tickIndexModulus := tickIndex % tickSpacing
	if tickIndexModulus < 0 {
		tickIndexModulus += tickSpacing
	}

	if tickIndexModulus != 0 {
		tickIndex = tickIndex - tickIndexModulus
	}

	// Defense-in-depth check to ensure that the tick index is within the authorized range
	// Should never get here.
	if tickIndex > types.MaxTick || tickIndex < types.MinInitializedTickV2 {
		return 0, types.TickIndexNotWithinBoundariesError{ActualTick: tickIndex, MinTick: types.MinInitializedTickV2, MaxTick: types.MaxTick}
	}

	return tickIndex, nil
}

// powTen treats negative exponents as 1/(10**|exponent|) instead of 10**-exponent
// This is because the osmomath.Dec.Power function does not support negative exponents
// func PowTenInternal(exponent int64) osmomath.Dec {
// 	if exponent >= 0 {
// 		return powersOfTen[exponent]
// 	}
// 	return negPowersOfTen[-exponent]
// }

func powTenBigDec(exponent int64) float64 {
	if exponent >= 0 {
		return bigPowersOfTen[exponent]
	}
	return bigNegPowersOfTen[-exponent]
}
