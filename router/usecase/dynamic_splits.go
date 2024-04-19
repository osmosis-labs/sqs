package usecase

import (
	"context"
	"errors"
	"fmt"

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

func getSplitQuote(ctx context.Context, routes []route.RouteImpl, tokenIn sdk.Coin) (domain.Quote, error) {
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

	bestSplit, err := findSplit(ctx, memo, routes, 0, tokenIn.Denom, tokenIn.Amount.ToLegacyDec(), totalIncrements, initialEmptySplit, initialEmptySplit)
	if err != nil {
		return nil, err
	}

	if bestSplit.amountOut.IsZero() {
		return nil, errors.New("amount out is zero, try increasing amount in")
	}

	for i := 0; i < 10; i++ {
		zero := memo[0]
		one := memo[1]

		zeroI := zero[uint8(i)]
		oneI := one[uint8(i)]

		fmt.Println("zero", zeroI)
		fmt.Println("one", oneI)

		fmt.Println("total", zeroI.Add(oneI))

		fmt.Printf("\n\n\n")
	}

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

		inAmount := tokenIn.Amount.ToLegacyDec().Mul(sdk.NewDec(int64(currentRouteIncrement))).Quo(sdk.NewDec(int64(totalIncrements))).TruncateInt()
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

	quote := &quoteImpl{
		AmountIn:  tokenIn,
		AmountOut: bestSplit.amountOut,
		Route:     resultRoutes,
	}

	return quote, nil
}

// Recurrence relation:
// // findSplit(currentIncrement, currentRoute) = max(estimate(currentRoute, tokeInAmt * currentIncrement / totalIncrements) + OptimalSplit(remainingIncrement - currentIncrement, remaining_routes[1:]))
func findSplit(ctx context.Context, memo []map[uint8]osmomath.Int, routes []route.RouteImpl, currentRouteIndex uint8, tokenInDenom string, tokenInAmount osmomath.Dec, remainingIncrements uint8, bestSplitSoFar, currentSplit split) (split, error) {
	// Current route index must be within range
	if currentRouteIndex >= uint8(len(routes)) {
		return split{}, fmt.Errorf("current route index (%d) is out of range (%d)", currentRouteIndex, len(routes))
	}

	currentRoute := routes[currentRouteIndex]

	// Base case: if this is the last route, consume all the remaining tokenIn
	if currentRouteIndex == uint8(len(routes))-1 {
		currentIncrement := remainingIncrements

		// Attempt to get memoized value.
		currentAmtOut, ok := memo[currentRouteIndex][currentIncrement]
		if !ok {
			coinOut, err := currentRoute.CalculateTokenOutByTokenIn(ctx, sdk.NewCoin(tokenInDenom, tokenInAmount.Mul(sdk.NewDec(int64(currentIncrement))).Quo(sdk.NewDec(int64(totalIncrements))).TruncateInt()))
			if err != nil {
				// Note that we should always return bestSplitSoFar if there is an error
				// since we silently skip the failing splits and want to preserve the context about bestSplitSoFar
				return bestSplitSoFar, err
			}

			if coinOut.Amount.IsNil() || coinOut.Amount.IsZero() {
				coinOut.Amount = osmomath.ZeroInt()
			}

			// Memoize
			memo[currentRouteIndex][currentIncrement] = coinOut.Amount
			currentAmtOut = coinOut.Amount
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

		// Attempt to get memoized value.
		currentAmtOut, ok := memo[currentRouteIndex][currentIncrement]
		if !ok {
			if currentIncrement == 0 {
				zeroResult := osmomath.ZeroInt()
				memo[currentRouteIndex][currentIncrement] = zeroResult
				currentAmtOut = zeroResult
			} else {
				coinOut, err := currentRoute.CalculateTokenOutByTokenIn(ctx, sdk.NewCoin(tokenInDenom, tokenInAmount.Mul(sdk.NewDec(int64(currentIncrement))).Quo(sdk.NewDec(int64(totalIncrements))).TruncateInt()))
				if err != nil {
					continue
				}

				if coinOut.Amount.IsNil() || coinOut.Amount.IsZero() {
					coinOut.Amount = osmomath.ZeroInt()
				}

				// Memoize
				memo[currentRouteIndex][currentIncrement] = coinOut.Amount
				currentAmtOut = coinOut.Amount
			}
		}

		// TODO: consider avoiding copy
		currentSplitCopy := split{}
		currentSplitCopy.routeIncrements = make([]int16, len(currentSplit.routeIncrements))
		copy(currentSplitCopy.routeIncrements, currentSplit.routeIncrements)
		currentSplitCopy.amountOut = currentSplit.amountOut.Add(currentAmtOut)
		currentSplitCopy.routeIncrements[currentRouteIndex] = int16(currentIncrement)

		// Recurse
		split, err := findSplit(ctx, memo, routes, currentRouteIndex+1, tokenInDenom, tokenInAmount, remainingIncrements-currentIncrement, bestSplitSoFar, currentSplitCopy)
		if err != nil {
			continue
		}

		// Update bestSplitSoFar
		if split.amountOut.GT(bestSplitSoFar.amountOut) {
			bestSplitSoFar = split
		}
	}

	return bestSplitSoFar, nil
}
