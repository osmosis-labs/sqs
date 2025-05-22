package pools

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ALTree/bigfloat"
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
func solveConstantFunctionInvariantBigFloat(
	tokenBalanceFixedBefore,
	tokenBalanceFixedAfter,
	tokenWeightFixed,
	tokenBalanceUnknownBefore,
	tokenWeightUnknown *big.Float,
) *big.Float {
	// weightRatio = (weightX/weightY)
	weightRatio := new(big.Float).Quo(tokenWeightFixed, tokenWeightUnknown)
	if weightRatio.IsInf() || weightRatio.Cmp(big.NewFloat(0)) == 0 {
		panic("weight ratio is zero")
	}

	// y = balanceXBefore/balanceXAfter
	y := new(big.Float).Quo(tokenBalanceFixedBefore, tokenBalanceFixedAfter)

	// amountY = balanceY * (1 - (y ^ weightRatio))
	yWeightRatio := bigfloat.Pow(y, weightRatio)
	if yWeightRatio.IsInf() {
		panic("yWeightRatio owerflow")
	}

	paranthetical := new(big.Float).Sub(oneDecBig, yWeightRatio)
	amountY := new(big.Float).Mul(paranthetical, tokenBalanceUnknownBefore)

	return amountY
}
