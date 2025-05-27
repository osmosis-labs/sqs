package math_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/router/usecase/pools/cl/math"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/types"
)

const (
	defaultTickSpacing = 100
)

var (
	// spot price - (10^(spot price exponent - 6 - 1))
	// Note we get spot price exponent by counting the number of digits in the max spot price and subtracting 1.
	closestPriceBelowMaxPriceDefaultTickSpacing = types.MaxSpotPrice.Sub(osmomath.NewDec(10).PowerMut(uint64(len(types.MaxSpotPrice.TruncateInt().String()) - 1 - int(-types.ExponentAtPriceOne) - 1)))
	// min tick + 10 ^ -expoentAtPriceOne
	closestTickAboveMinPriceDefaultTickSpacing = osmomath.NewInt(types.MinInitializedTick).Add(osmomath.NewInt(10).ToLegacyDec().Power(uint64(types.ExponentAtPriceOne * -1)).TruncateInt())

	smallestBigDec = osmomath.SmallestBigDec()
	bigOneDec      = osmomath.OneDec()
	bigTenDec      = osmomath.NewBigDec(10)
)

// use following equations to test testing vectors using sage
// geometricExponentIncrementDistanceInTicks(exponentAtPriceOne) = (9 * (10^(-exponentAtPriceOne)))
// geometricExponentDelta(tickIndex, exponentAtPriceOne)  = floor(tickIndex / geometricExponentIncrementDistanceInTicks(exponentAtPriceOne))
// exponentAtCurrentTick(tickIndex, exponentAtPriceOne) = exponentAtPriceOne + geometricExponentDelta(tickIndex, exponentAtPriceOne)
// currentAdditiveIncrementInTicks(tickIndex, exponentAtPriceOne) = pow(10, exponentAtCurrentTick(tickIndex, exponentAtPriceOne))
// numAdditiveTicks(tickIndex, exponentAtPriceOne) = tickIndex - (geometricExponentDelta(tickIndex, exponentAtPriceOne) * geometricExponentIncrementDistanceInTicks(exponentAtPriceOne)
// price(tickIndex, exponentAtPriceOne) = pow(10, geometricExponentDelta(tickIndex, exponentAtPriceOne)) +
// (numAdditiveTicks(tickIndex, exponentAtPriceOne) * currentAdditiveIncrementInTicks(tickIndex, exponentAtPriceOne))
func TestTickToSqrtPrice(t *testing.T) {
	testCases := map[string]struct {
		tickIndex     int64
		expectedPrice osmomath.BigDec
		expectedError error
	}{
		"Ten billionths cent increments at the millionths place: 1": {
			tickIndex:     -51630100,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.0000033699"),
		},
		"Ten billionths cent increments at the millionths place: 2": {
			tickIndex:     -51630000,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.0000033700"),
		},
		"One millionths cent increments at the hundredths place: 1": {
			tickIndex:     -11999800,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.070002"),
		},
		"One millionths cent increments at the hundredths place: 2": {
			tickIndex:     -11999700,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.070003"),
		},
		"One hundred thousandth cent increments at the tenths place: 1": {
			tickIndex:     -999800,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.90002"),
		},
		"One hundred thousandth cent increments at the tenths place: 2": {
			tickIndex:     -999700,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.90003"),
		},
		"One ten thousandth cent increments at the ones place: 1": {
			tickIndex:     1000000,
			expectedPrice: osmomath.MustNewBigDecFromStr("2"),
		},
		"One dollar increments at the ten thousands place: 2": {
			tickIndex:     1000100,
			expectedPrice: osmomath.MustNewBigDecFromStr("2.0001"),
		},
		"One thousandth cent increments at the tens place: 1": {
			tickIndex:     9200100,
			expectedPrice: osmomath.MustNewBigDecFromStr("12.001"),
		},
		"One thousandth cent increments at the tens place: 2": {
			tickIndex:     9200200,
			expectedPrice: osmomath.MustNewBigDecFromStr("12.002"),
		},
		"One cent increments at the hundreds place: 1": {
			tickIndex:     18320100,
			expectedPrice: osmomath.MustNewBigDecFromStr("132.01"),
		},
		"One cent increments at the hundreds place: 2": {
			tickIndex:     18320200,
			expectedPrice: osmomath.MustNewBigDecFromStr("132.02"),
		},
		"Ten cent increments at the thousands place: 1": {
			tickIndex:     27732100,
			expectedPrice: osmomath.MustNewBigDecFromStr("1732.10"),
		},
		"Ten cent increments at the thousands place: 2": {
			tickIndex:     27732200,
			expectedPrice: osmomath.MustNewBigDecFromStr("1732.20"),
		},
		"Dollar increments at the ten thousands place: 1": {
			tickIndex:     36073200,
			expectedPrice: osmomath.MustNewBigDecFromStr("10732"),
		},
		"Dollar increments at the ten thousands place: 2": {
			tickIndex:     36073300,
			expectedPrice: osmomath.MustNewBigDecFromStr("10733"),
		},
		"Max tick and min k": {
			tickIndex:     342000000,
			expectedPrice: types.MaxSpotPriceBigDec,
		},
		"tickIndex is MinInitializedTickV1": {
			tickIndex: types.MinInitializedTick,
			// 1 order of magnitude below min spot price of 10^-12 + 6 orders of magnitude smaller
			// to account for exponent at price one of -6.
			expectedPrice: types.MinSpotPriceBigDec,
		},
		"max sqrt price, max tick -> max spot price": {
			tickIndex:     types.MaxTick,
			expectedPrice: types.MaxSpotPriceBigDec,
		},
		"tickIndex is MinCurrentTickV1": {
			tickIndex: types.MinCurrentTick,
			// 1 order of magnitude below min spot price of 10^-12 + 6 orders of magnitude smaller
			// to account for exponent at price one of -6.
			expectedPrice: types.MinSpotPriceBigDec.Sub(osmomath.BigDecFromDec(osmomath.SmallestDec()).Quo(bigTenDec)),
		},
		"tickIndex is MinInitializedTickV2": {
			tickIndex:     types.MinInitializedTickV2,
			expectedPrice: types.MinSpotPriceV2,
		},
		"tickIndex is MinCurrentTickV2": {
			tickIndex:     types.MinCurrentTickV2,
			expectedPrice: types.MinSpotPriceV2,
		},
		"tickIndex is MinInitializedTick + 1 ULP": {
			tickIndex:     types.MinInitializedTickV2 + 1,
			expectedPrice: types.MinSpotPriceV2.Add(smallestBigDec),
		},
		"tickIndex is MinInitializedTick + 2 ULP": {
			tickIndex:     types.MinInitializedTickV2 + 2,
			expectedPrice: types.MinSpotPriceV2.Add(smallestBigDec.MulInt64(2)),
		},
		"error: tickIndex less than minimum": {
			tickIndex:     types.MinCurrentTickV2 - 1,
			expectedError: types.TickIndexMinimumError{MinTick: types.MinCurrentTickV2},
		},
		"error: tickIndex greater than maximum": {
			tickIndex:     types.MaxTick + 1,
			expectedError: types.TickIndexMaximumError{MaxTick: types.MaxTick},
		},
		"Gyen <> USD, tick -20594000 -> price 0.0074060": {
			tickIndex:     -20594000,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.007406000000000000"),
		},
		"Gyen <> USD, tick -20594000 + 100 -> price 0.0074061": {
			tickIndex:     -20593900,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.007406100000000000"),
		},
		"Spell <> USD, tick -29204000 -> price 0.00077960": {
			tickIndex:     -29204000,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.000779600000000000"),
		},
		"Spell <> USD, tick -29204000 + 100 -> price 0.00077961": {
			tickIndex:     -29203900,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.000779610000000000"),
		},
		"Atom <> Osmo, tick -12150000 -> price 0.068500": {
			tickIndex:     -12150000,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.068500000000000000"),
		},
		"Atom <> Osmo, tick -12150000 + 100 -> price 0.068501": {
			tickIndex:     -12149900,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.068501000000000000"),
		},
		"Boot <> Osmo, tick 64576000 -> price 25760000": {
			tickIndex:     64576000,
			expectedPrice: osmomath.MustNewBigDecFromStr("25760000"),
		},
		"Boot <> Osmo, tick 64576000 + 100 -> price 25760000": {
			tickIndex:     64576100,
			expectedPrice: osmomath.MustNewBigDecFromStr("25761000"),
		},
		"BTC <> USD, tick 38035200 -> price 30352": {
			tickIndex:     38035200,
			expectedPrice: osmomath.MustNewBigDecFromStr("30352"),
		},
		"BTC <> USD, tick 38035200 + 100 -> price 30353": {
			tickIndex:     38035300,
			expectedPrice: osmomath.MustNewBigDecFromStr("30353"),
		},
		"SHIB <> USD, tick -44821000 -> price 0.000011790": {
			tickIndex:     -44821000,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.00001179"),
		},
		"SHIB <> USD, tick -44821100 + 100 -> price 0.000011791": {
			tickIndex:     -44820900,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.000011791"),
		},
		"ETH <> BTC, tick -12104000 -> price 0.068960": {
			tickIndex:     -12104000,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.068960"),
		},
		"ETH <> BTC, tick -121044000 + 1 -> price 0.068961": {
			tickIndex:     -12103900,
			expectedPrice: osmomath.MustNewBigDecFromStr("0.068961"),
		},
		"one tick spacing interval smaller than max sqrt price, max tick neg six - 100 -> one tick spacing interval smaller than max sqrt price": {
			tickIndex:     types.MaxTick - 100,
			expectedPrice: osmomath.MustNewBigDecFromStr("99999000000000000000000000000000000000"),
		},
		"max sqrt price, max tick neg six -> max spot price": {
			tickIndex:     types.MaxTick,
			expectedPrice: types.MaxSpotPriceBigDec,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			sqrtPrice, err := math.TickToSqrtPrice(tc.tickIndex)
			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError.Error(), err.Error())
				return
			}
			require.NoError(t, err)

			var expectedSqrtPrice osmomath.BigDec
			if tc.expectedPrice.LT(types.MinSpotPriceBigDec) {
				expectedSqrtPrice = osmomath.MustMonotonicSqrtBigDec(tc.expectedPrice)
			} else {
				expectedSqrtPrice = osmomath.BigDecFromDec(osmomath.MustMonotonicSqrt(tc.expectedPrice.Dec()))
				require.NoError(t, err)
			}

			require.Equal(t, expectedSqrtPrice.String(), sqrtPrice.String())
		})
	}
}

func TestTickToPrice_SuccessCases(t *testing.T) {
	testCases := map[string]struct {
		tickIndex     int64
		expectedPrice osmomath.BigDec
		expectedErr   error
	}{
		"tick index is Max tick": {
			tickIndex:     types.MaxTick,
			expectedPrice: osmomath.BigDecFromDec(types.MaxSpotPrice),
		},
		"tick index is between Min tick V1 and Max tick": {
			tickIndex:     123456,
			expectedPrice: osmomath.OneBigDec().Add(osmomath.NewBigDec(123456).Mul(osmomath.NewBigDecWithPrec(1, 6))),
		},
		"tick index is V1 MinInitializedTick": {
			tickIndex:     types.MinInitializedTick,
			expectedPrice: osmomath.BigDecFromDec(types.MinSpotPrice),
		},
		"tick index is V1 MinCurrentTick": {
			tickIndex:     types.MinCurrentTick,
			expectedPrice: osmomath.BigDecFromDec(types.MinSpotPrice).Sub(osmomath.NewBigDecWithPrec(1, 13+(-types.ExponentAtPriceOne))),
		},
		"tick index is V2 MinInitializedTick": {
			tickIndex:     types.MinInitializedTickV2,
			expectedPrice: types.MinSpotPriceV2,
		},
		"tick index is V2 MinCurrentTickV2": {
			tickIndex:     types.MinCurrentTickV2,
			expectedPrice: types.MinSpotPriceV2,
		},
		"tick index is V2 MinInitializedTick + 1": {
			tickIndex:     types.MinInitializedTickV2 + 1,
			expectedPrice: types.MinSpotPriceV2.Add(smallestBigDec),
		},
		"tick index is V2 MinInitializedTick + 2": {
			tickIndex:     types.MinInitializedTickV2 + 2,
			expectedPrice: types.MinSpotPriceV2.Add(smallestBigDec).Add(smallestBigDec),
		},
		// Computed in Python:
		// geometricExponentIncrementDistanceInTicks = 9000000
		// tickIndex = -9000000 * 18 - 123456
		// geometricExponentDelta = tickIndex // geometricExponentIncrementDistanceInTicks + 1 # add one because Python is a floor division when Go is truncation towards zero.
		// exponentAtCurrentTick = -6 + geometricExponentDelta - 1
		// currentAdditiveIncrementInTicks = 10**exponentAtCurrentTick
		// numAdditiveTicks = tickIndex - (geometricExponentDelta * geometricExponentIncrementDistanceInTicks)
		// 10**geometricExponentDelta + numAdditiveTicks * currentAdditiveIncrementInTicks
		// 9.876544e-19
		"tick index is between V2 MinInitializedTick and V1 MinInitializedTick": {
			tickIndex:     -9000000*18 - 123456,
			expectedPrice: osmomath.NewBigDecWithPrec(9876544, 6+19), // 6 for number of digits after, 18 for geometric multiplier and 1 for negative ticks
		},
	}
	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			price, err := math.TickToPrice(tc.tickIndex)
			if tc.expectedErr != nil {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			require.Equal(t, tc.expectedPrice.String(), price.String())
		})
	}
}

func TestTickToPrice_ErrorCases(t *testing.T) {
	testCases := map[string]struct {
		tickIndex int64
	}{
		"tick index is greater than max tick": {
			tickIndex: types.MaxTick + 1,
		},
		"tick index is less than min tick": {
			tickIndex: types.MinCurrentTickV2 - 1,
		},
	}
	for name, tc := range testCases {
		tc := tc

		t.Run(name, func(t *testing.T) {
			_, err := math.TickToPrice(tc.tickIndex)
			require.Error(t, err)
		})
	}
}
