package math

import (
	"github.com/osmosis-labs/osmosis/osmomath"

	"github.com/ericlagergren/decimal"
	"github.com/ericlagergren/decimal/math" // nolint: staticcheck
)

var (
	osmomathBigOneDec = mustDecFromString(osmomath.NewBigDec(1).String())
	osmomathBigTenDec = mustDecFromString(osmomath.NewBigDec(10).String())
	bigPowersOfTen    []*decimal.Big
	bigNegPowersOfTen []*decimal.Big
)

// Set precision multipliers
func init() {
	bigNegPowersOfTen = make([]*decimal.Big, osmomath.BigDecPrecision+1)
	for i := 0; i <= osmomath.BigDecPrecision; i++ {
		pow := math.Pow(new(decimal.Big), osmomathBigTenDec, decimal.New(int64(i), 0))
		quo := new(decimal.Big).Quo(osmomathBigOneDec, pow)
		bigNegPowersOfTen[i] = quo
	}
	// 10^308 < osmomath.MaxInt < 10^309
	bigPowersOfTen = make([]*decimal.Big, 309)
	for i := 0; i <= 308; i++ {
		bigPowersOfTen[i] = math.Pow(new(decimal.Big), osmomathBigTenDec, decimal.New(int64(i), 0))
	}
}

// mustDecFromString parses a string as a decimal.Big and panics if it fails.
func mustDecFromString(s string) *decimal.Big {
	d, ok := new(decimal.Big).SetString(s)
	if !ok {
		panic("decFromString: failed to parse string as decimal")
	}
	return d
}
