package math

import (
	"math"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/types"
)

var (
	sdkOneDec      = osmomath.OneDec().MustFloat64()
	sdkTenDec      = osmomath.NewDec(10).MustFloat64()
	powersOfTen    []float64
	negPowersOfTen []float64

	osmomathBigOneDec = osmomath.NewBigDec(1)
	osmomathBigTenDec = osmomath.NewBigDec(10)
	bigPowersOfTen    []float64
	bigNegPowersOfTen []float64

	// 9 * 10^(-types.ExponentAtPriceOne), where types.ExponentAtPriceOne is non-positive and is s.t.
	// this answer fits well within an int64.
	geometricExponentIncrementDistanceInTicks = 9 * osmomath.NewDec(10).PowerMut(uint64(-types.ExponentAtPriceOne)).TruncateInt64()
)

// Builds metadata for every additive tickspacing exponent, namely:
// * what is the first price this tick spacing applies to
// * what is the first tick this applies for
// * (saves on pre-compute) what is the additive increment per tick.
//
// This would be stored in a map, keyed by:
// 0 => (1.00, 10^(types.ExponentAtPriceOne), 0)
// 1 => (10, 10^(1 + types.ExponentAtPriceOne), 9 * types.ExponentAtPriceOne)
// 2 => (100, 10^(2 + types.ExponentAtPriceOne), 9 * (types.ExponentAtPriceOne + 1))
// -1 => (0.1, 10^(types.ExponentAtPriceOne - 1), 9 * (types.ExponentAtPriceOne - 1))
type tickExpIndexData struct {
	// if price < initialPrice, we are not in this exponent range.
	initialPrice float64
	// if price >= maxPrice, we are not in this exponent range.
	maxPrice float64
	// TODO: Change to normal Dec, if min spot price increases.
	// additive increment per tick here.
	additiveIncrementPerTick float64
	// the tick that corresponds to initial price
	initialTick int64
}

var tickExpCache map[int64]*tickExpIndexData = make(map[int64]*tickExpIndexData)

func buildTickExpCache() {
	// build positive indices first
	maxPrice := osmomathBigOneDec.MustFloat64()
	curExpIndex := int64(0)
	for maxPrice < MaxSpotPrice { // LT
		tickExpCache[curExpIndex] = &tickExpIndexData{
			// price range 10^curExpIndex to 10^(curExpIndex + 1). (10, 100)
			initialPrice:             osmomathBigTenDec.PowerInteger(uint64(curExpIndex)).MustFloat64(),
			maxPrice:                 osmomathBigTenDec.PowerInteger(uint64(curExpIndex + 1)).MustFloat64(),
			additiveIncrementPerTick: powTenBigDec(types.ExponentAtPriceOne + curExpIndex),
			initialTick:              geometricExponentIncrementDistanceInTicks * curExpIndex,
		}
		maxPrice = tickExpCache[curExpIndex].maxPrice
		curExpIndex += 1
	}

	minPrice := osmomathBigOneDec.MustFloat64()
	curExpIndex = -1
	minSpotPrice := osmomath.NewBigDecWithPrec(1, 30).MustFloat64() // 10^-30
	for minPrice > minSpotPrice {                                   // GT
		tickExpCache[curExpIndex] = &tickExpIndexData{
			// price range 10^curExpIndex to 10^(curExpIndex + 1). (0.001, 0.01)
			initialPrice:             powTenBigDec(curExpIndex),
			maxPrice:                 powTenBigDec(curExpIndex + 1),
			additiveIncrementPerTick: powTenBigDec(types.ExponentAtPriceOne + curExpIndex),
			initialTick:              geometricExponentIncrementDistanceInTicks * curExpIndex,
		}
		minPrice = tickExpCache[curExpIndex].initialPrice
		curExpIndex -= 1
	}
}

// Set precision multipliers
// Set precision multipliers
func init() {
	// Initialize negPowersOfTen using float64
	negPowersOfTen = make([]float64, osmomath.DecPrecision+1)
	one := 1.0
	ten := 10.0

	for i := 0; i <= osmomath.DecPrecision; i++ {
		// Calculate 10^i using bigfloat.Pow
		exponent := float64(i)
		power := math.Pow(ten, exponent)

		// Calculate 1 / 10^i
		negPowersOfTen[i] = one / power
	}

	// 10^77 < osmomath.MaxInt < 10^78
	powersOfTen = make([]float64, 77)
	for i := 0; i <= 76; i++ {
		// Calculate 10^i using bigfloat.Pow
		exponent := float64(i)
		powersOfTen[i] = math.Pow(ten, exponent)
	}

	// Initialize bigNegPowersOfTen using float64
	bigNegPowersOfTen = make([]float64, osmomath.BigDecPrecision+1)
	for i := 0; i <= osmomath.BigDecPrecision; i++ {
		// Calculate 10^i using bigfloat.Pow
		exponent := float64(i)
		power := math.Pow(ten, exponent)

		// Calculate 1 / 10^i
		bigNegPowersOfTen[i] = one / power
	}

	// 10^308 < osmomath.MaxInt < 10^309
	bigPowersOfTen = make([]float64, 309)
	for i := 0; i <= 308; i++ {
		// Calculate 10^i using bigfloat.Pow
		exponent := float64(i)
		bigPowersOfTen[i] = math.Pow(ten, exponent)
	}

	buildTickExpCache()
}
