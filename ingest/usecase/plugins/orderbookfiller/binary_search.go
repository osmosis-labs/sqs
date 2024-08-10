package orderbookfiller

import (
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"go.uber.org/zap"
)

// nolint: unused
func (o *orderbookFillerIngestPlugin) binarySearchOptimalUSDCValue(ctx blockContext, minUSDC, maxUSDC osmomath.BigDec, denomIn, denomOut string, canonicalOrderbookPoolId uint64) (osmomath.BigDec, osmomath.Int, error) {
	// 100 / 2^14 = 0.006 ~ cent precision
	const maxRecursionDepth = 14
	return o.binarySearchOptimalUSDCValueRecusive(ctx, minUSDC, maxUSDC, denomIn, denomOut, canonicalOrderbookPoolId, maxRecursionDepth)
}

// nolint: unused
func (o *orderbookFillerIngestPlugin) binarySearchOptimalUSDCValueRecusive(ctx blockContext, minUSDC, maxUSDC osmomath.BigDec, denomIn, denomOut string, canonicalOrderbookPoolId uint64, maxRecursionDepth int) (osmomath.BigDec, osmomath.Int, error) {
	defer func() {
		if r := recover(); r != nil {
			o.logger.Error("panic processing orderbook", zap.Any("recover", r))
		}
	}()

	// Reached precision threshold or max recursion depth
	epsilon := osmomath.MustNewBigDecFromStr("0.01") // Precision threshold
	if maxUSDC.Sub(minUSDC).LT(epsilon) || maxRecursionDepth == 0 {
		amountIn, amountOut, _, _, err := o.tryUSDAmountIn(ctx, denomIn, denomOut, minUSDC, canonicalOrderbookPoolId)
		if err != nil {
			return osmomath.BigDec{}, osmomath.Int{}, err
		}

		if amountOut.IsNil() {
			return osmomath.BigDec{}, osmomath.Int{}, fmt.Errorf("amount out is nil")
		}

		diff := amountOut.Sub(amountIn)
		if !diff.IsPositive() {
			return osmomath.BigDec{}, osmomath.Int{}, fmt.Errorf("diff is not positive")
		}

		return minUSDC, amountOut.Sub(amountIn), nil
	}

	midUSDC := minUSDC.Add(maxUSDC).Quo(osmomath.NewBigDec(2))

	lowerValue, lowerDiff, lowerErr := o.binarySearchOptimalUSDCValueRecusive(ctx, minUSDC, midUSDC, denomIn, denomOut, canonicalOrderbookPoolId, maxRecursionDepth-1)

	// Note: domain.OrderbookNotEnoughLiquidityToCompleteSwapError
	// If lower range errored due to not enough liqudity, does not make sense to search in higher range
	if lowerErr != nil && strings.Contains(lowerErr.Error(), "not enough liquidity to complete swap in pool") {
		return osmomath.BigDec{}, osmomath.Int{}, lowerErr
	}

	amountInMid, err := o.usdcToDenomVlaue(denomIn, midUSDC.Dec(), ctx.prices)
	if err != nil {
		return osmomath.BigDec{}, osmomath.Int{}, err
	}

	midAmountIn, midAmountOut, _, midErr := o.estimateArb(ctx, sdk.Coin{Denom: denomIn, Amount: amountInMid}, denomOut, canonicalOrderbookPoolId)
	// If mid range errored due to not enough liqudity, does not make sense to search in higher range
	if midErr != nil && strings.Contains(midErr.Error(), "not enough liquidity to complete swap in pool") {
		return lowerValue, lowerDiff, lowerErr
	}

	upperValue, upperDiff, upperErr := o.binarySearchOptimalUSDCValueRecusive(ctx, midUSDC, maxUSDC, denomIn, denomOut, canonicalOrderbookPoolId, maxRecursionDepth-1)
	// If higher range erorred due to not enough liqudity, choose the best value between lower and mid
	if upperErr != nil && strings.Contains(upperErr.Error(), "not enough liquidity to complete swap in pool") {
		if midErr != nil {
			return lowerValue, lowerDiff, lowerErr
		}

		return o.makeChoice(lowerValue, lowerDiff, lowerErr, midUSDC, midAmountOut.Sub(midAmountIn), midErr)
	}

	// Otherwise,
	// Choose the best value between lower and upper
	topValue, topDiff, topErr := o.makeChoice(lowerValue, lowerDiff, lowerErr, upperValue, upperDiff, upperErr)

	// If mid errored, return top value
	if midErr != nil {
		return topValue, topDiff, topErr
	}

	// Otherwise, compare mid to top choise between lower and upper ranges
	return o.makeChoice(midUSDC, midAmountOut.Sub(midAmountIn), midErr, topValue, topDiff, topErr)
}

// nolint: unused
func (o *orderbookFillerIngestPlugin) makeChoice(usdcA osmomath.BigDec, diffA osmomath.Int, errA error, usdcB osmomath.BigDec, diffB osmomath.Int, errB error) (osmomath.BigDec, osmomath.Int, error) {
	// If both errored, return error
	if errA != nil && errB != nil {
		return osmomath.BigDec{}, osmomath.Int{}, fmt.Errorf("failed to find a working amount")
	}

	// If A errored, choose B
	if errA != nil {
		if diffB.IsPositive() {
			return usdcB, diffB, errB
		}
		return osmomath.BigDec{}, osmomath.Int{}, fmt.Errorf("one errored, another diff is non-positive")
	}

	// If B errored, choose A
	if errB != nil {
		if diffA.IsPositive() {
			return usdcA, diffA, errA
		}
		return osmomath.BigDec{}, osmomath.Int{}, fmt.Errorf("one errored, another diff is non-positive")
	}

	if !diffA.IsPositive() && !diffB.IsPositive() {
		return osmomath.BigDec{}, osmomath.Int{}, fmt.Errorf("both amounts are non-positive when making a choice")
	}

	// Choose top value
	if diffA.GT(diffB) {
		return usdcA, diffA, errA
	}

	return usdcB, diffB, errB
}

// nolint: unused, revive
func (o *orderbookFillerIngestPlugin) tryUSDAmountIn(ctx blockContext, denomIn, denomOut string, midUSDC osmomath.BigDec, canonicalOrderbookPoolId uint64) (osmomath.Int, osmomath.Int, bool, bool, error) {
	var shouldUpdateMaxToMid = false
	var shouldUpdateMinToMid = false

	amountIn, err := o.usdcToDenomVlaue(denomIn, midUSDC.Dec(), ctx.prices)
	if err != nil {
		return osmomath.Int{}, osmomath.Int{}, shouldUpdateMinToMid, shouldUpdateMaxToMid, err
	}

	if amountIn.IsZero() {
		// The token value ends up truncating to zero, go to e higher range
		shouldUpdateMinToMid = true
		return osmomath.Int{}, osmomath.Int{}, shouldUpdateMinToMid, shouldUpdateMaxToMid, nil
	}

	coinIn := sdk.Coin{Denom: denomIn, Amount: amountIn}

	originalAmountIn, amountOut, _, err := o.estimateArb(ctx, coinIn, denomOut, canonicalOrderbookPoolId)
	if err != nil {
		// Note: domain.OrderbookNotEnoughLiquidityToCompleteSwapError
		if strings.Contains(err.Error(), "not enough liquidity to complete swap in pool") {
			// Not enough liquidity to complete the swap
			// Search a lower range
			shouldUpdateMaxToMid = true
			return osmomath.Int{}, osmomath.Int{}, shouldUpdateMinToMid, shouldUpdateMaxToMid, nil
		}

		return osmomath.ZeroInt(), osmomath.ZeroInt(), shouldUpdateMinToMid, shouldUpdateMaxToMid, err
	}

	return originalAmountIn, amountOut, shouldUpdateMinToMid, shouldUpdateMaxToMid, nil
}
