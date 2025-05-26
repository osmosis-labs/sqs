package swapstrategy

import (
	"math"

	storetypes "cosmossdk.io/store/types"
)

// zeroForOneStrategy implements the swapStrategy interface.
// This implementation assumes that we are swapping token 0 for
// token 1 and performs calculations accordingly.
//
// With this strategy, we are moving to the left of the current
// tick index and square root price.
type zeroForOneStrategy struct {
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
//   - amountZeroInRemaining is the amount of token zero in remaining to be swapped. This amount is fully consumed
//     if sqrt price target is not reached. In that case, the returned amountZeroIn is the amount remaining given.
//     Otherwise, the returned amountIn will be smaller than amountZeroInRemaining given.
//
// Returns:
//   - sqrtPriceNext is the next sqrt price. It equals sqrt price target if target is reached. Otherwise, it is in-between sqrt price current and target.
//   - amountZeroIn is the amount of token zero in consumed. It equals amountZeroInRemaining if target is reached. Otherwise, it is less than amountZeroInRemaining.
//   - amountOutComputed is the amount of token out computed. It is the amount of token out to return to the user.
//   - spreadRewardChargeTotal is the total spread reward charge. The spread reward is charged on the amount of token in.
//
// ZeroForOne details:
// - zeroForOneStrategy assumes moving to the left of the current square root price.
func (s zeroForOneStrategy) ComputeSwapWithinBucketOutGivenIn(sqrtPriceCurrent, sqrtPriceTarget, liquidity, amountZeroInRemaining float64) (float64, float64, float64, float64) {
	// Estimate the amount of token zero needed until the target sqrt price is reached.
	amountZeroIn := CalcAmount0Delta(liquidity, sqrtPriceTarget, sqrtPriceCurrent, true) // N.B.: if this is false, causes infinite loop

	// Calculate sqrtPriceNext on the amount of token remaining after spread reward.
	oneMinusTakerFee := s.getOneMinusSpreadFactor()
	amountZeroInRemainingLessSpreadReward := amountZeroInRemaining * oneMinusTakerFee

	var sqrtPriceNext float64
	// If have more of the amount remaining after spread reward than estimated until target,
	// bound the next sqrtPriceNext by the target sqrt price.
	if amountZeroInRemainingLessSpreadReward >= amountZeroIn {
		sqrtPriceNext = sqrtPriceTarget
	} else {
		// Otherwise, compute the next sqrt price based on the amount remaining after spread reward.
		sqrtPriceNext = GetNextSqrtPriceFromAmount0InRoundingUp(sqrtPriceCurrent, liquidity, amountZeroInRemainingLessSpreadReward)
	}

	hasReachedTarget := floatEqual(sqrtPriceTarget, sqrtPriceNext)

	// If the sqrt price target was not reached, recalculate how much of the amount remaining after spread reward was needed
	// to complete the swap step. This implies that some of the amount remaining after spread reward is left over after the
	// current swap step.
	if !hasReachedTarget {
		amountZeroIn = CalcAmount0Delta(liquidity, sqrtPriceNext, sqrtPriceCurrent, true) // N.B.: if this is false, causes infinite loop
	}

	// Calculate the amount of the other token given the sqrt price range.
	amountOneOut := CalcAmount1Delta(liquidity, sqrtPriceNext, sqrtPriceCurrent, false)

	// Round up to charge user more in pool's favor.
	amountZeroInFinal := math.Ceil(amountZeroIn*1e18) / 1e18

	// Handle spread rewards.
	// Note that spread reward is always charged on the amount in.
	spreadRewardChargeTotal := computeSpreadRewardChargePerSwapStepOutGivenIn(hasReachedTarget, amountZeroInFinal, amountZeroInRemaining, s.spreadFactor, s.getSpfOverOneMinusSpf)

	// Round down amount out to give user less in pool's favor.
	return sqrtPriceNext, amountZeroInFinal, math.Floor(amountOneOut*1e18) / 1e18, spreadRewardChargeTotal
}

// ComputeSwapWithinBucketInGivenOut calculates the next sqrt price, the amount of token out consumed, the amount in to charge to the user for requested out, and total spread reward charge on token in.
// This assumes swapping over a single bucket where the liqudiity stays constant until we cross the next initialized tick of the next bucket.
// Parameters:
//   - sqrtPriceCurrent is the current sqrt price.
//   - sqrtPriceTarget is the target sqrt price computed with GetSqrtTargetPrice(). It must be one of:
//     1. Next initialized tick sqrt price.
//     2. Sqrt price limit representing price impact protection.
//   - liquidity is the amount of liquidity between the sqrt price current and sqrt price target.
//   - amountOneRemainingOut is the amount of token one out remaining to be swapped to estimate how much of token zero in is needed to be charged.
//     This amount is fully consumed if sqrt price target is not reached. In that case, the returned amountOneOut is the amount remaining given.
//     Otherwise, the returned amountOneOut will be smaller than amountOneRemainingOut given.
//
// Returns:
//   - sqrtPriceNext is the next sqrt price. It equals sqrt price target if target is reached. Otherwise, it is in-between sqrt price current and target.
//   - amountOneOut is the amount of token one out consumed. It equals amountOneRemainingOut if target is reached. Otherwise, it is less than amountOneRemainingOut.
//   - amountZeroIn is the amount of token zero in computed. It is the amount of token in to charge to the user for the desired amount out.
//   - spreadRewardChargeTotal is the total spread reward charge. The spread reward is charged on the amount of token in.
//
// ZeroForOne details:
// - zeroForOneStrategy assumes moving to the left of the current square root price.
func (s zeroForOneStrategy) ComputeSwapWithinBucketInGivenOut(sqrtPriceCurrent, sqrtPriceTarget, liquidity, amountOneRemainingOut float64) (float64, float64, float64, float64) {
	// Estimate the amount of token one needed until the target sqrt price is reached.
	amountOneOut := CalcAmount1Delta(liquidity, sqrtPriceTarget, sqrtPriceCurrent, false)

	// Calculate sqrtPriceNext on the amount of token remaining. Note that the
	// spread reward is not charged as amountRemaining is amountOut, and we only charge spread reward on
	// amount in.
	var sqrtPriceNext float64
	// If have more of the amount remaining after spread reward than estimated until target,
	// bound the next sqrtPriceNext by the target sqrt price.
	if amountOneRemainingOut >= amountOneOut {
		sqrtPriceNext = sqrtPriceTarget
	} else {
		// Otherwise, compute the next sqrt price based on the amount remaining after spread reward.
		sqrtPriceNext = GetNextSqrtPriceFromAmount1OutRoundingDown(sqrtPriceCurrent, liquidity, amountOneRemainingOut)
	}

	hasReachedTarget := floatEqual(sqrtPriceTarget, sqrtPriceNext)

	// If the sqrt price target was not reached, recalculate how much of the amount remaining after spread reward was needed
	// to complete the swap step. This implies that some of the amount remaining after spread reward is left over after the
	// current swap step.
	if !hasReachedTarget {
		amountOneOut = CalcAmount1Delta(liquidity, sqrtPriceNext, sqrtPriceCurrent, false)
	}

	// Calculate the amount of the other token given the sqrt price range.
	amountZeroIn := CalcAmount0Delta(liquidity, sqrtPriceNext, sqrtPriceCurrent, true)

	// Round up to charge user more in pool's favor.
	amountZeroInFinal := math.Ceil(amountZeroIn*1e18) / 1e18

	// Handle spread rewards.
	// Note that spread reward is always charged on the amount in.
	spreadRewardChargeTotal := computeSpreadRewardChargeFromAmountIn(amountZeroInFinal, s.getSpfOverOneMinusSpf())

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
	if amountOneOut > amountOneRemainingOut {
		amountOneOut = amountOneRemainingOut
	}

	// Round down amount out to give user less in pool's favor.
	return sqrtPriceNext, math.Floor(amountOneOut*1e18) / 1e18, amountZeroInFinal, spreadRewardChargeTotal
}

func (s zeroForOneStrategy) getOneMinusSpreadFactor() float64 {
	if s.oneMinusSpreadFactor == 0 {
		s.oneMinusSpreadFactor = oneDec - s.spreadFactor
	}
	return s.oneMinusSpreadFactor
}

func (s zeroForOneStrategy) getSpfOverOneMinusSpf() float64 {
	if s.spfOverOneMinusSpf == 0 {
		oneMinusSpf := s.getOneMinusSpreadFactor()
		if oneMinusSpf == 0 {
			panic("division by zero: oneMinusSpreadFactor is zero")
		}
		s.spfOverOneMinusSpf = math.Ceil((s.spreadFactor/oneMinusSpf)*1e18) / 1e18
	}
	return s.spfOverOneMinusSpf
}
