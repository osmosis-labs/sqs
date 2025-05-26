package swapstrategy

import (
	"fmt"
	"math"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/types"

	storetypes "cosmossdk.io/store/types"
)

// swapStrategy defines the interface for computing a swap.
// There are 2 implementations of this interface:
// - zeroForOneStrategy to provide implementations when swapping token 0 for token 1.
// - oneForZeroStrategy to provide implementations when swapping token 1 for token 0.
type SwapStrategy interface {
	// ComputeSwapWithinBucketOutGivenIn calculates the next sqrt price, the amount of token in consumed, the amount out to return to the user, and total spread reward charge on token in.
	// This assumes swapping over a single bucket where the liqudiity stays constant until we cross the next initialized tick of the next bucket.
	// Parameters:
	//   * sqrtPriceCurrent is the current sqrt price.
	//   * sqrtPriceTarget is the target sqrt price computed with GetSqrtTargetPrice(). It must be one of:
	//       - Next initialized tick sqrt price.
	//       - Sqrt price limit representing price impact protection.
	//   * liquidity is the amount of liquidity between the sqrt price current and sqrt price target.
	//   * amountRemainingIn is the amount of token in remaining to be swapped. This amount is fully consumed
	//   if sqrt price target is not reached. In that case, the returned amountInConsumed is the amount remaining given.
	//   Otherwise, the returned amountInConsumed will be smaller than amountRemainingIn given.
	// Returns:
	//   * sqrtPriceNext is the next sqrt price. It equals sqrt price target if target is reached. Otherwise, it is in-between sqrt price current and target.
	//   * amountInConsumed is the amount of token in consumed. It equals amountRemainingIn if target is reached. Otherwise, it is less than amountRemainingIn.
	//   * amountOutComputed is the amount of token out computed. It is the amount of token out to return to the user.
	//   * spreadRewardChargeTotal is the total spread reward charge. The spread reward is charged on the amount of token in.
	// See oneForZeroStrategy or zeroForOneStrategy for implementation details.
	ComputeSwapWithinBucketOutGivenIn(sqrtPriceCurrent, sqrtPriceTarget, liquidity, amountRemainingIn float64) (sqrtPriceNext, amountInConsumed, amountOutComputed, spreadRewardChargeTotal float64)
	// ComputeSwapWithinBucketInGivenOut calculates the next sqrt price, the amount of token out consumed, the amount in to charge to the user for requested out, and total spread reward charge on token in.
	// This assumes swapping over a single bucket where the liqudiity stays constant until we cross the next initialized tick of the next bucket.
	// Parameters:
	//   * sqrtPriceCurrent is the current sqrt price.
	//   * sqrtPriceTarget is the target sqrt price computed with GetSqrtTargetPrice(). It must be one of:
	//       - Next initialized tick sqrt price.
	//       - Sqrt price limit representing price impact protection.
	//   * liquidity is the amount of liquidity between the sqrt price current and sqrt price target.
	//   * amountRemainingOut is the amount of token out remaining to be swapped to estimate how much of token in is needed to be charged.
	//   This amount is fully consumed if sqrt price target is not reached. In that case, the returned amountOutConsumed is the amount remaining given.
	//   Otherwise, the returned amountOutConsumed will be smaller than amountRemainingOut given.
	// Returns:
	//   * sqrtPriceNext is the next sqrt price. It equals sqrt price target if target is reached. Otherwise, it is in-between sqrt price current and target.
	//   * amountOutConsumed is the amount of token out consumed. It equals amountRemainingOut if target is reached. Otherwise, it is less than amountRemainingOut.
	//   * amountInComputed is the amount of token in computed. It is the amount of token in to charge to the user for the desired amount out.
	//   * spreadRewardChargeTotal is the total spread reward charge. The spread reward is charged on the amount of token in.
	// See oneForZeroStrategy or zeroForOneStrategy for implementation details.
	ComputeSwapWithinBucketInGivenOut(sqrtPriceCurrent, sqrtPriceTarget, liquidity, amountRemainingOut float64) (sqrtPriceNext, amountOutConsumed, amountInComputed, spreadRewardChargeTotal float64)
}

var (
	oneDec = 1.0
)

// New returns a swap strategy based on the provided zeroForOne parameter
// with sqrtPriceLimit for the maximum square root price until which to perform
// the swap and the stor key of the module that stores swap data.
func New(zeroForOne bool, sqrtPriceLimit float64, storeKey storetypes.StoreKey, spreadFactor float64) SwapStrategy {
	if zeroForOne {
		return &zeroForOneStrategy{sqrtPriceLimit: sqrtPriceLimit, storeKey: storeKey, spreadFactor: spreadFactor}
	}
	return &oneForZeroStrategy{sqrtPriceLimit: sqrtPriceLimit, storeKey: storeKey, spreadFactor: spreadFactor}
}

// GetPriceLimit returns the price limit based on which token is being swapped in.
// If zero in for one out, the price is decreasing. Therefore, min spot price is the limit.
// If one in for zero out, the price is increasing. Therefore, max spot price is the limit.
func GetPriceLimit(zeroForOne bool) float64 {
	if zeroForOne {
		return 1e-12 // MinSpotPrice equivalent
	}
	return 1e12 // MaxSpotPrice equivalent
}

// GetSqrtPriceLimit returns sqrt price limit from price limit and swap strategy.
// If price limit is zero and strategy is zero for one, min sqrt price is returned.
// If price limit is zero and strategy is one for zero, max sqrt price is returned.
// If price limit is greater than MaxSpotPrice, an error is returned.
// Otherwise, if price limit is less that MinSpotPrice, a big decimal sqrt function
// is used to get the sqrt price limit. Otherwise, a decimal sqrt function is used.
// The sqrt function choice strategy applies to both zero for one and one for zero.
// Such a choice is made to keep state-compatibility with the original at-launch
// price range.
func GetSqrtPriceLimit(priceLimit float64, zeroForOne bool) (float64, error) {
	if priceLimit == 0 {
		if zeroForOne {
			return 1e-6, nil // MinSqrtPrice equivalent
		}
		return 1e6, nil // MaxSqrtPrice equivalent
	}

	minSpotPriceV2 := 1e-12
	maxSpotPrice := 1e12
	if priceLimit < minSpotPriceV2 || priceLimit > maxSpotPrice {
		// Convert float64 back to osmomath types for error reporting
		priceLimitDec := osmomath.MustNewDecFromStr(fmt.Sprintf("%.18f", priceLimit))
		minSpotPriceV2Dec := osmomath.MustNewDecFromStr(fmt.Sprintf("%.18f", minSpotPriceV2))
		maxSpotPriceDec := osmomath.MustNewDecFromStr(fmt.Sprintf("%.18f", maxSpotPrice))
		return 0, types.PriceBoundError{ProvidedPrice: osmomath.BigDecFromDec(priceLimitDec), MinSpotPrice: osmomath.BigDecFromDec(minSpotPriceV2Dec), MaxSpotPrice: maxSpotPriceDec}
	}

	// Calculate sqrt price using standard math.Sqrt
	sqrtPriceLimit := math.Sqrt(priceLimit)
	if math.IsNaN(sqrtPriceLimit) || math.IsInf(sqrtPriceLimit, 0) {
		return 0, fmt.Errorf("invalid sqrt price calculation: %f", priceLimit)
	}

	return sqrtPriceLimit, nil
}
