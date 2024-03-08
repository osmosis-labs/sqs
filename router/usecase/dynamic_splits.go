package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/route"
)

type split struct {
	routeIncrements []int16
	amountOut       osmomath.Int
}

const totalIncrements = uint8(10)

func (r *Router) GetSplitQuote(ctx context.Context, routes []route.RouteImpl, tokenIn sdk.Coin) (domain.Quote, error) {
	// Routes must be non-empty
	if len(routes) == 0 {
		return nil, errors.New("no routes")
	}
	// If only one route, return the best single route quote
	if len(routes) == 1 {
		route := routes[0]
		coinOut, err := route.CalculateTokenOutByTokenIn(ctx, tokenIn)
		if err != nil {
			return nil, err
		}

		quote := &quoteImpl{
			AmountIn:  tokenIn,
			AmountOut: coinOut.Amount,
			Route: []domain.SplitRoute{&RouteWithOutAmount{
				RouteImpl: route,
				OutAmount: coinOut.Amount,
				InAmount:  tokenIn.Amount,
			}},
		}

		return quote, nil
	}

	if r.splitsCache != nil {
		// Get splits from cache
		tokenOutDenom := routes[0].GetTokenOutDenom()

		splitsObj, found := r.splitsCache.Get(formatRouteCacheKey(tokenIn.Denom, tokenOutDenom))

		// Note, we naively rely on the fact that the splits cache is consistent with the routes.
		// It is possible that the splits cache is stale and the routes have changed.
		// In that case, we will recompute them.
		if found {

			splits, ok := splitsObj.([]osmomath.Dec)
			if !ok {
				return nil, fmt.Errorf("splits cache is not of type []osmomath.Dec, token in (%s), token out (%s)", tokenIn.Denom, tokenOutDenom)
			}

			if len(splits) == len(routes) {
				amountInDec := tokenIn.Amount.ToLegacyDec()

				totalAmountOut := osmomath.ZeroInt()

				resultRoutes := make([]domain.SplitRoute, 0, len(routes))

				for i, split := range splits {
					if split.IsZero() {
						continue
					}

					inAmount := amountInDec.Mul(split).TruncateInt()

					coinOut, err := routes[i].CalculateTokenOutByTokenIn(ctx, sdk.NewCoin(tokenIn.Denom, inAmount))
					if err != nil {
						return nil, err
					}

					totalAmountOut = totalAmountOut.Add(coinOut.Amount)

					resultRoutes = append(resultRoutes, &RouteWithOutAmount{
						RouteImpl: routes[i],
						InAmount:  inAmount,
						OutAmount: coinOut.Amount,
					})
				}

				return &quoteImpl{
					AmountIn:  tokenIn,
					AmountOut: totalAmountOut,
					Route:     resultRoutes,
				}, nil
			}
		}
	}

	memo := make([]map[uint8]osmomath.Int, len(routes))
	for i := range memo {
		memo[i] = make(map[uint8]osmomath.Int, totalIncrements)
	}

	routeIncrements := make([]int16, len(routes))
	for j := range routes {
		routeIncrements[j] = -1
	}

	initialEmptySplit := split{
		routeIncrements: routeIncrements,
		amountOut:       osmomath.ZeroInt(),
	}

	bestSplit, err := r.findSplit(ctx, memo, routes, 0, tokenIn, totalIncrements, initialEmptySplit, initialEmptySplit)
	if err != nil {
		return nil, err
	}

	if bestSplit.amountOut.IsZero() {
		return nil, errors.New("amount out is zero, try increasing amount in")
	}

	inAmountProportions := make([]osmomath.Dec, 0, len(routes))

	totalIncrementsInSplits := uint8(0)
	resultRoutes := make([]domain.SplitRoute, 0, len(routes))
	totalAmoutOutFromSplits := osmomath.ZeroInt()
	for i, currentRouteIncrement := range bestSplit.routeIncrements {
		currentRoute := routes[i]

		currentRouteIndex := uint8(i)

		if currentRouteIncrement < 0 {
			return nil, fmt.Errorf("best increment for route %d is negative", currentRouteIndex)
		}

		currentRouteAmtOut, ok := memo[currentRouteIndex][uint8(currentRouteIncrement)]
		if currentRouteIncrement > 0 && !ok {
			return nil, fmt.Errorf("route %d not found in memo", currentRouteIndex)
		}

		currentInAmountProportion := sdk.NewDec(int64(currentRouteIncrement)).QuoInt64Mut(int64(totalIncrements))

		inAmountProportions = append(inAmountProportions, currentInAmountProportion)

		inAmount := tokenIn.Amount.ToLegacyDec().MulMut(currentInAmountProportion).TruncateInt()
		outAmount := currentRouteAmtOut

		isAmountInNilOrZero := inAmount.IsNil() || inAmount.IsZero()
		isAmountOutNilOrZero := outAmount.IsNil() || outAmount.IsZero()
		if isAmountInNilOrZero && isAmountOutNilOrZero {
			continue
		}

		if isAmountInNilOrZero {
			return nil, fmt.Errorf("in amount is zero when out is not (%s), route index (%d)", outAmount, currentRouteIndex)
		}

		if isAmountOutNilOrZero {
			return nil, fmt.Errorf("out amount is zero when in is not (%s), route index (%d)", inAmount, currentRouteIndex)
		}

		resultRoutes = append(resultRoutes, &RouteWithOutAmount{
			RouteImpl: currentRoute,
			InAmount:  inAmount,
			OutAmount: currentRouteAmtOut,
		})

		totalIncrementsInSplits += uint8(currentRouteIncrement)
		totalAmoutOutFromSplits = totalAmoutOutFromSplits.Add(currentRouteAmtOut)
	}

	if !totalAmoutOutFromSplits.Equal(bestSplit.amountOut) {
		return nil, fmt.Errorf("total amount out from splits (%s) does not equal actual amount out (%s)", totalAmoutOutFromSplits, bestSplit.amountOut)
	}

	// This may happen if one of the routes is consistently returning 0 amount out for all increments.
	// TODO: we may want to remove this check so that we get the best quote.
	if totalIncrementsInSplits != totalIncrements {
		return nil, fmt.Errorf("total increments (%d) does not match expected total increments (%d)", totalIncrements, totalIncrements)
	}

	// Cache propotions
	if r.splitsCache != nil {
		tokenOutDenom := routes[0].GetTokenOutDenom()
		r.splitsCache.Set(formatRouteCacheKey(tokenIn.Denom, tokenOutDenom), inAmountProportions, time.Duration(r.config.RankedRouteCacheExpirySeconds)*time.Second)
	}

	quote := &quoteImpl{
		AmountIn:  tokenIn,
		AmountOut: bestSplit.amountOut,
		Route:     resultRoutes,
	}

	return quote, nil
}

// Recurrence relation:
// // findSplit(currentIncrement, currentRoute) = max(estimate(currentRoute, tokeInAmt * currentIncrement / totalIncrements) + OptimalSplit(remainingIncrement - currentIncrement, remaining_routes[1:]))
func (r *Router) findSplit(ctx context.Context, memo []map[uint8]osmomath.Int, routes []route.RouteImpl, currentRouteIndex uint8, tokenIn sdk.Coin, remainingIncrements uint8, bestSplitSoFar, currentSplit split) (split, error) {
	// Current route index must be within range
	if currentRouteIndex >= uint8(len(routes)) {
		return split{}, fmt.Errorf("current route index (%d) is out of range (%d)", currentRouteIndex, len(routes))
	}

	tokenInAmountDec := tokenIn.Amount.ToLegacyDec()
	currentRoute := routes[currentRouteIndex]

	// Base case: if this is the last route, consume all the remaining tokenIn
	if currentRouteIndex == uint8(len(routes))-1 {
		currentIncrement := remainingIncrements

		// Attempt to get memoized value.
		currentAmtOut, err := getAmountOut(ctx, currentRoute, currentRouteIndex, memo, currentIncrement, tokenInAmountDec, tokenIn.Denom)
		if err != nil {
			// Note that we should always return bestSplitSoFar if there is an error
			// since we silently skip the failing splits and want to preserve the context about bestSplitSoFar
			return bestSplitSoFar, err
		}

		currentSplit.amountOut = currentSplit.amountOut.Add(currentAmtOut)

		if currentSplit.amountOut.GT(bestSplitSoFar.amountOut) {
			// update current split with the increment of the current route.
			currentSplit.routeIncrements[currentRouteIndex] = int16(currentIncrement)
			return currentSplit, nil
		}

		return bestSplitSoFar, nil
	}

	// TODO: start from highest and exit early
	for currentIncrement := uint8(0); currentIncrement <= remainingIncrements; currentIncrement++ {
		currentAmtOut, err := getAmountOut(ctx, currentRoute, currentRouteIndex, memo, currentIncrement, tokenInAmountDec, tokenIn.Denom)
		if err != nil {
			continue
		}

		// TODO: consider avoiding copy
		currentSplitCopy := split{}
		currentSplitCopy.routeIncrements = make([]int16, len(currentSplit.routeIncrements))
		copy(currentSplitCopy.routeIncrements, currentSplit.routeIncrements)
		currentSplitCopy.amountOut = currentSplit.amountOut.Add(currentAmtOut)
		currentSplitCopy.routeIncrements[currentRouteIndex] = int16(currentIncrement)

		// Recurse
		bestSplitSoFar, err = r.findSplit(ctx, memo, routes, currentRouteIndex+1, tokenIn, remainingIncrements-currentIncrement, bestSplitSoFar, currentSplitCopy)
		if err != nil {
			continue
		}
	}

	return bestSplitSoFar, nil
}

// getAmountOut returns the amount out for the given route and increment.
// If the result is already present in the memo, it returns the memoized value.
// Otherwise, it calculates the amount out and memoizes it by mutating the memo.
// Returns error if the amount out cannot be calculated.
// Otherwise, returns nil.
func getAmountOut(ctx context.Context, route route.RouteImpl, memoRouteIndex uint8, memo []map[uint8]osmomath.Int, currentIncrement uint8, totalAmountIn osmomath.Dec, tokenInDenom string) (amtOut osmomath.Int, err error) {
	if currentIncrement == 0 {
		zeroResult := osmomath.ZeroInt()
		memo[memoRouteIndex][currentIncrement] = zeroResult
		return zeroResult, nil
	}

	currentAmtOut, ok := memo[memoRouteIndex][currentIncrement]

	currentRatio := osmomath.NewDec(int64(currentIncrement)).Quo(osmomath.NewDec(int64(totalIncrements)))
	currentTokenAmountIn := currentRatio.MulMut(totalAmountIn)
	amtIn := currentTokenAmountIn.TruncateInt()

	if !ok {
		coinOut, err := route.CalculateTokenOutByTokenIn(ctx, sdk.NewCoin(tokenInDenom, amtIn))
		if err != nil {
			return osmomath.Int{}, err
		}

		if coinOut.Amount.IsNil() || coinOut.Amount.IsZero() {
			zeroResult := osmomath.ZeroInt()
			memo[memoRouteIndex][currentIncrement] = zeroResult
			return zeroResult, nil
		}

		currentAmtOut = coinOut.Amount

		// Memoize
		memo[memoRouteIndex][currentIncrement] = currentAmtOut
	}

	return currentAmtOut, nil
}
