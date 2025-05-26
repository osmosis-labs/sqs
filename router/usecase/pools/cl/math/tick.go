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

func powTenBigDec(exponent int64) float64 {
	if exponent >= 0 {
		return bigPowersOfTen[exponent]
	}
	return bigNegPowersOfTen[-exponent]
}
