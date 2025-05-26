package balancer

import (
	"math"
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
	tokenWeightUnknown float64,
) float64 {
	weightRatio := tokenWeightFixed / tokenWeightUnknown
	if math.IsInf(weightRatio, 0) || weightRatio == 0 {
		panic("weight ratio is zero or overflow")
	}

	// y = balanceXBefore/balanceXAfter
	y := tokenBalanceFixedBefore / tokenBalanceFixedAfter

	// amountY = balanceY * (1 - (y ^ weightRatio))
	yWeightRatio := math.Pow(y, weightRatio)
	if math.IsInf(yWeightRatio, 0) {
		panic("yWeightRatio overflow")
	}

	paranthetical := oneDec - yWeightRatio
	amountY := paranthetical * tokenBalanceUnknownBefore

	return amountY
}
