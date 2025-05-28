package swapstrategy

func ComputeSpreadRewardChargePerSwapStepOutGivenIn(hasReachedTarget bool, amountIn, amountSpecifiedRemaining, spreadFactor float64) float64 {
	spreadFactorOverOneMinusSpreadFactorGetter := func() float64 {
		return spreadFactor / (1 - spreadFactor)
	}
	return computeSpreadRewardChargePerSwapStepOutGivenIn(hasReachedTarget, amountIn, amountSpecifiedRemaining, spreadFactor, spreadFactorOverOneMinusSpreadFactorGetter)
}

func ComputeSpreadRewardChargeFromAmountIn(amountIn, spreadFactor float64) float64 {
	return computeSpreadRewardChargeFromAmountIn(amountIn, spreadFactor/(1-spreadFactor))
}
