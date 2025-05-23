package balancer

import (
	"math/big"

	"github.com/ALTree/bigfloat"
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
	tokenWeightUnknown *big.Float,
) *big.Float {
	// weightRatio = (weightX/weightY)
	weightRatio := new(big.Float).Quo(tokenWeightFixed, tokenWeightUnknown)
	if weightRatio.IsInf() || weightRatio.Cmp(big.NewFloat(0)) == 0 {
		panic("weight ratio is zero or overflow")
	}

	// y = balanceXBefore/balanceXAfter
	y := new(big.Float).Quo(tokenBalanceFixedBefore, tokenBalanceFixedAfter)

	// amountY = balanceY * (1 - (y ^ weightRatio))
	yWeightRatio := bigfloat.Pow(y, weightRatio)
	if yWeightRatio.IsInf() {
		panic("yWeightRatio overflow")
	}

	paranthetical := new(big.Float).Sub(oneDec, yWeightRatio)
	amountY := new(big.Float).Mul(paranthetical, tokenBalanceUnknownBefore)

	return amountY
}
