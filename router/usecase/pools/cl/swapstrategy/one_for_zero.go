package swapstrategy

import (
	"math"

	storetypes "cosmossdk.io/store/types"
)

// oneForZeroStrategy implements the swapStrategy interface.
// This implementation assumes that we are swapping token 1 for
// token 0 and performs calculations accordingly.
//
// With this strategy, we are moving to the right of the current
// tick index and square root price.
type oneForZeroStrategy struct {
	sqrtPriceLimit float64
	storeKey       storetypes.StoreKey
	spreadFactor   float64

	// oneMinusSpreadFactor is 1 - spreadFactor
	oneMinusSpreadFactor float64
	// spfOverOneMinusSpf is spreadFactor / (1 - spreadFactor)
	spfOverOneMinusSpf float64
}

// ComputeSwapWithinBucketOutGivenIn calculates the next sqrt price, the amount of token in consumed, the amount out to return to the user, and total spread reward charge on token in.
// Parameters:
//   - sqrtPriceCurrent is the current sqrt price.
//   - sqrtPriceTarget is the target sqrt price computed with GetSqrtTargetPrice(). It must be one of:
//     1. Next tick sqrt price.
//     2. Sqrt price limit representing price impact protection.
//   - liquidity is the amount of liquidity between the sqrt price current and sqrt price target.
//   - amountOneRemainingIn is the amount of token one in remaining to be swapped. This amount is fully consumed
//     if sqrt price target is not reached. In that case, the returned amountOne is the amount remaining given.
//     Otherwise, the returned amountOneIn will be smaller than amountOneRemainingIn given.
//
// Returns:
//   - sqrtPriceNext is the next sqrt price. It equals sqrt price target if target is reached. Otherwise, it is in-between sqrt price current and target.
//   - amountOneIn is the amount of token in consumed. It equals amountRemainingIn if target is reached. Otherwise, it is less than amountOneRemainingIn.
//   - amountZeroOut the amount of token out computed. It is the amount of token out to return to the user.
//   - spreadRewardChargeTotal is the total spread reward charge. The spread reward is charged on the amount of token in.
//
// OneForZero details:
// - oneForZeroStrategy assumes moving to the right of the current square root price.
func (s oneForZeroStrategy) ComputeSwapWithinBucketOutGivenIn(sqrtPriceCurrent, sqrtPriceTarget, liquidity, amountOneInRemaining float64) (float64, float64, float64, float64) {
	// Estimate the amount of token one needed until the target sqrt price is reached.
	amountOneIn := CalcAmount1Delta(liquidity, sqrtPriceTarget, sqrtPriceCurrent, true)

	// Calculate sqrtPriceNext on the amount of token remaining after spread reward.
	oneMinusTakerFee := s.getOneMinusSpreadFactor()
	amountOneInRemainingLessSpreadReward := amountOneInRemaining * oneMinusTakerFee

	var sqrtPriceNext float64
	// If have more of the amount remaining after spread reward than estimated until target,
	// bound the next sqrtPriceNext by the target sqrt price.
	if amountOneInRemainingLessSpreadReward >= amountOneIn {
		sqrtPriceNext = sqrtPriceTarget
	} else {
		// Otherwise, compute the next sqrt price based on the amount remaining after spread reward.
		sqrtPriceNext = GetNextSqrtPriceFromAmount1InRoundingDown(sqrtPriceCurrent, liquidity, amountOneInRemainingLessSpreadReward)
	}

	hasReachedTarget := floatEqual(sqrtPriceTarget, sqrtPriceNext)

	// If the sqrt price target was not reached, recalculate how much of the amount remaining after spread reward was needed
	// to complete the swap step. This implies that some of the amount remaining after spread reward is left over after the
	// current swap step.
	if !hasReachedTarget {
		amountOneIn = CalcAmount1Delta(liquidity, sqrtPriceNext, sqrtPriceCurrent, true) // N.B.: if this is false, causes infinite loop
	}

	// Calculate the amount of the other token given the sqrt price range.
	amountZeroOut := CalcAmount0Delta(liquidity, sqrtPriceNext, sqrtPriceCurrent, false)

	// Round up to charge user more in pool's favor.
	amountInDecFinal := math.Ceil(amountOneIn*1e18) / 1e18

	// Handle spread rewards.
	// Note that spread reward is always charged on the amount in.
	spreadRewardChargeTotal := computeSpreadRewardChargePerSwapStepOutGivenIn(hasReachedTarget, amountInDecFinal, amountOneInRemaining, s.spreadFactor, s.getSpfOverOneMinusSpf)

	// Round down amount out to give user less in pool's favor.
	return sqrtPriceNext, amountInDecFinal, math.Floor(amountZeroOut*1e18) / 1e18, spreadRewardChargeTotal
}

// ComputeSwapWithinBucketInGivenOut calculates the next sqrt price, the amount of token out consumed, the amount in to charge to the user for requested out, and total spread reward charge on token in.
// This assumes swapping over a single bucket where the liqudiity stays constant until we cross the next initialized tick of the next bucket.
// Parameters:
//   - sqrtPriceCurrent is the current sqrt price.
//   - sqrtPriceTarget is the target sqrt price computed with GetSqrtTargetPrice(). It must be one of:
//     1. Next initialized tick sqrt price.
//     2. Sqrt price limit representing price impact protection.
//   - liquidity is the amount of liquidity between the sqrt price current and sqrt price target.
//   - amountZeroRemainingOut is the amount of token zero out remaining to be swapped to estimate how much of token one in is needed to be charged.
//     This amount is fully consumed if sqrt price target is not reached. In that case, the returned amountOut is the amount zero remaining given.
//     Otherwise, the returned amountOut will be smaller than amountZeroRemainingOut given.
//
// Returns:
//   - sqrtPriceNext is the next sqrt price. It equals sqrt price target if target is reached. Otherwise, it is in-between sqrt price current and target.
//   - amountZeroOut is the amount of token zero out consumed. It equals amountZeroRemainingOut if target is reached. Otherwise, it is less than amountZeroRemainingOut.
//   - amountIn is the amount of token in computed. It is the amount of token one in to charge to the user for the desired amount out.
//   - spreadRewardChargeTotal is the total spread reward charge. The spread reward is charged on the amount of token in.
//
// OneForZero details:
// - oneForZeroStrategy assumes moving to the right of the current square root price.
func (s oneForZeroStrategy) ComputeSwapWithinBucketInGivenOut(sqrtPriceCurrent, sqrtPriceTarget, liquidity, amountZeroRemainingOut float64) (float64, float64, float64, float64) {
	// Estimate the amount of token zero needed until the target sqrt price is reached.
	// N.B.: contrary to out given in, we do not round up because we do not want to exceed the initial amount out at the end.
	amountZeroOut := CalcAmount0Delta(liquidity, sqrtPriceTarget, sqrtPriceCurrent, false)

	// Calculate sqrtPriceNext on the amount of token remaining. Note that the
	// spread reward is not charged as amountRemaining is amountOut, and we only charge spread reward on
	// amount in.
	var sqrtPriceNext float64
	// If have more of the amount remaining after spread reward than estimated until target,
	// bound the next sqrtPriceNext by the target sqrt price.
	if amountZeroRemainingOut >= amountZeroOut {
		sqrtPriceNext = sqrtPriceTarget
	} else {
		// Otherwise, compute the next sqrt price based on the amount remaining after spread reward.
		sqrtPriceNext = GetNextSqrtPriceFromAmount0OutRoundingUp(sqrtPriceCurrent, liquidity, amountZeroRemainingOut)
	}

	hasReachedTarget := floatEqual(sqrtPriceTarget, sqrtPriceNext)

	// If the sqrt price target was not reached, recalculate how much of the amount remaining after spread reward was needed
	// to complete the swap step. This implies that some of the amount remaining after spread reward is left over after the
	// current swap step.
	if !hasReachedTarget {
		// N.B.: contrary to out given in, we do not round up because we do not want to exceed the initial amount out at the end.
		amountZeroOut = CalcAmount0Delta(liquidity, sqrtPriceNext, sqrtPriceCurrent, false)
	}

	// Calculate the amount of the other token given the sqrt price range.
	amountOneIn := CalcAmount1Delta(liquidity, sqrtPriceNext, sqrtPriceCurrent, true)

	// Round up to charge user more in pool's favor.
	amountOneInFinal := math.Ceil(amountOneIn*1e18) / 1e18

	// Handle spread rewards.
	// Note that spread reward is always charged on the amount in.
	spreadRewardChargeTotal := computeSpreadRewardChargeFromAmountIn(amountOneInFinal, s.getSpfOverOneMinusSpf())

	// Cap the output amount to not exceed the remaining output amount.
	// The reason why we must do this for in given out and NOT out given in is the following:
	// When swapping for exact out while not reaching sqrtPriceTarget, we calculate  sqrtPriceNext from the
	// amountRemainingOut. While calculating it, we round sqrtPriceNext in the direction opposite from the sqrtPriceCurrent.
	// This is because we need to move the price up enough so that we get the desired output amount out.
	// From newly calculate sqrtPriceNext, we then re-calculate the amountOut actually consumed. In certain cases, this
	// recalculation might lead to a slightly greater amount than remaining due to sqrtPriceNext having been rounded in
	// the opposite direction of the sqrtPriceCurrent. Therefore, we force the amountOut consumed to equal to amountRemaining.
	// This is acceptable since the former is calculated from the latter, and the only possible source of difference is rounding.
	// Going back to the exact in swap, when calculating the sqrtPriceNext, we round it in the direction of the sqrtPriceCurrent.
	// As a result, this rounding error should not be possible in its case.
	if amountZeroOut > amountZeroRemainingOut {
		amountZeroOut = amountZeroRemainingOut
	}

	// Round down amount out to give user less in pool's favor.
	return sqrtPriceNext, math.Floor(amountZeroOut*1e18) / 1e18, amountOneInFinal, spreadRewardChargeTotal
}

func (s oneForZeroStrategy) getOneMinusSpreadFactor() float64 {
	if s.oneMinusSpreadFactor == 0 {
		s.oneMinusSpreadFactor = oneDec - s.spreadFactor
	}
	return s.oneMinusSpreadFactor
}

func (s oneForZeroStrategy) getSpfOverOneMinusSpf() float64 {
	if s.spfOverOneMinusSpf == 0 {
		oneMinusSpf := s.getOneMinusSpreadFactor()
		if oneMinusSpf == 0 {
			panic("division by zero: oneMinusSpreadFactor is zero")
		}
		s.spfOverOneMinusSpf = math.Ceil((s.spreadFactor/oneMinusSpf)*1e18) / 1e18
	}
	return s.spfOverOneMinusSpf
}
