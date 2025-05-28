package math

import (
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	clmath "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/math"
	"github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/types"

	"github.com/ericlagergren/decimal"
)

var MaxSpotPrice = mustDecFromString(types.MaxSpotPrice.String())

var oneBigDec = decimal.New(1, 0)

var (
	MinSpotPriceV2BigDec = mustDecFromString(types.MinSpotPriceV2.String())
	MaxSpotPriceBigDec   = mustDecFromString(types.MaxSpotPriceBigDec.String())
)

// TickToSqrtPrice returns the sqrtPrice given a tickIndex
// If tickIndex is zero, the function returns osmomath.OneDec().
// It is the combination of calling TickToPrice followed by Sqrt.
func TickToSqrtPrice(tickIndex int64) (osmomath.BigDec, error) {
	priceDec, err := TickToPrice(tickIndex)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Format the priceDec to a string with appropriate precision besed on the tickIndex.
	// See below for the explanation of the precision.
	var precision int
	if tickIndex >= types.MinInitializedTick {
		precision = osmomath.DecPrecision
	} else {
		precision = osmomath.BigDecPrecision
	}

	priceFloatStr := fmt.Sprintf(fmt.Sprintf("%%.%df", precision), priceDec)

	priceBigDec, err := osmomath.NewBigDecFromStr(priceFloatStr)
	if err != nil {
		return osmomath.BigDec{}, fmt.Errorf("failed to convert string to BigDec: %w", err)
	}

	// N.B. at launch, we only supported price range
	// of [tick(10^-12), tick(MaxSpotPrice)].
	// To maintain backwards state-compatibility, we use the original
	// math based on 18 precision decimal on the at the launch tick range.
	if tickIndex >= types.MinInitializedTick {
		// It is acceptable to truncate here as TickToPrice() function converts
		// from osmomath.Dec to osmomath.BigDec before returning specifically for this range.
		// As a result, there is no data loss.
		price := priceBigDec.Dec()

		sqrtPrice, err := osmomath.MonotonicSqrtMut(price)
		if err != nil {
			return osmomath.BigDec{}, err
		}
		return osmomath.BigDecFromDecMut(sqrtPrice), nil
	}

	// For the newly extended range of [tick(MinSpotPriceV2), MinInitializedTick), we use the new math
	// based on 36 precision decimal.
	sqrtPrice, err := osmomath.MonotonicSqrtBigDec(priceBigDec)
	if err != nil {
		return osmomath.BigDec{}, err
	}
	return sqrtPrice, nil
}

// TickToPrice returns the price given a tickIndex
// If tickIndex is zero, the function returns osmomath.OneDec().
func TickToPrice(tickIndex int64) (*decimal.Big, error) {
	if tickIndex == 0 {
		return oneBigDec, nil
	}

	// N.B. We special case MinInitializedTickV2 and MinCurrentTickV2 since MinInitializedTickV2
	// is the first one that requires taking 10 to the exponent of (-31 + -6) = -37
	// Given BigDec's precision of 36, that cannot be supported.
	// The fact that MinInitializedTickV2 and MinCurrentTickV2 translate to the same
	// price is acceptable since MinCurrentTickV2 cannot be initialized.
	if tickIndex == types.MinInitializedTickV2 || tickIndex == types.MinCurrentTickV2 {
		return MinSpotPriceV2BigDec, nil
	}

	numAdditiveTicks, geometricExponentDelta, err := clmath.TickToAdditiveGeometricIndices(tickIndex)
	if err != nil {
		return nil, err
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
	unscaledPriceBigDec := decimal.New(unscaledPrice, 0)
	price := new(decimal.Big).Mul(pw, unscaledPriceBigDec)

	// defense in depth, this logic would not be reached due to use having checked if given tick is in between
	// min tick and max tick.
	if price.Cmp(MaxSpotPriceBigDec) > 0 || price.Cmp(MinSpotPriceV2BigDec) < 0 {
		return nil, types.PriceBoundError{} // TODO
		// return nil, cltypes.PriceBoundError{ProvidedPrice: price, MinSpotPrice: cltypes.MinSpotPriceV2, MaxSpotPrice: cltypes.MaxSpotPrice}
	}
	return price, nil
}

func powTenBigDec(exponent int64) *decimal.Big {
	if exponent >= 0 {
		return bigPowersOfTen[exponent]
	}
	return bigNegPowersOfTen[-exponent]
}
