package pools

import (
	"fmt"
	"math"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// solveConstantFunctionInvariant solves the constant function of an AMM
// that determines the relationship between the differences of two sides
// of assets inside the pool.
// For fixed balanceXBefore, balanceXAfter, weightX, balanceY, weightY,
// we could deduce the balanceYDelta, calculated by:
// balanceYDelta = balanceY * (1 - (balanceXBefore/balanceXAfter)^(weightX/weightY))
// balanceYDelta is positive when the balance liquidity decreases.
// balanceYDelta is negative when the balance liquidity increases.
//
// panics if tokenWeightUnknown is 0.
func solveConstantFunctionInvariant(
	tokenBalanceFixedBefore,
	tokenBalanceFixedAfter,
	tokenWeightFixed,
	tokenBalanceUnknownBefore,
	tokenWeightUnknown osmomath.Dec,
) osmomath.Dec {
	// weightRatio = (weightX/weightY)
	weightRatio := tokenWeightFixed.Quo(tokenWeightUnknown)

	// y = balanceXBefore/balanceXAfter
	y := tokenBalanceFixedBefore.Quo(tokenBalanceFixedAfter)

	// amountY = balanceY * (1 - (y ^ weightRatio))
	yWeightRatio := math.Pow(y.MustFloat64(), weightRatio.MustFloat64())
	if math.IsInf(yWeightRatio, 0) || math.IsNaN(yWeightRatio) {
		panic("constant-function invariant: overflow while exponentiating y ^ weightRatio")
	}

	yToWeightRatio := osmomath.MustNewDecFromStr(fmt.Sprintf("%v", yWeightRatio))
	paranthetical := oneDec.Sub(yToWeightRatio)

	amountY := paranthetical.MulMut(tokenBalanceUnknownBefore)
	return amountY
}
