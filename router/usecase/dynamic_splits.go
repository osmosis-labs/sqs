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
	routeIncrements []uint8
	amountOut       osmomath.Int
}

const totalIncrements = uint8(10)

// getSplitQuote returns the best quote for the given routes and tokenIn.
// It uses dynamic programming to find the optimal split of the tokenIn among the routes.
// The algorithm is based on the knapsack problem.
// The time complexity is O(n * m), where n is the number of routes and m is the totalIncrements.
// The space complexity is O(n * m).
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

	// proportions[x][j] stores the proportion of tokens used for the j-th
	// route that leads to the optimal value at each state. The proportions slice,
	// essentially, records the decision made at each step.
	proportions := make([][]uint8, totalIncrements+1)
	// dp stores the maximum output values.
	dp := make([][]osmomath.Int, totalIncrements+1)

	// Step 1: initialize tables
	for i := 0; i < int(totalIncrements+1); i++ {
		dp[i] = make([]osmomath.Int, len(routes)+1)

		dp[i][0] = zero

		proportions[i] = make([]uint8, len(routes)+1)
	}

	// Initialize the first column with 0
	for j := 0; j <= len(routes); j++ {
		dp[0][j] = zero
	}

	inAmountDec := tokenIn.Amount.ToLegacyDec()

	// This function computes the inAmountIncrement for a given proportion p.
	// It caches the result on the stack to avoid recomputing it.
	computeAndCacheInAmountIncrement := func(p uint8) osmomath.Int {
		inAmountIncrement := osmomath.Int{}
		return func() osmomath.Int {
			// If the inAmountIncrement has already been computed, return the cached value.
			// Otherwise, compute the value and cache it.
			if inAmountIncrement.IsNil() {
				inAmountIncrement = sdk.NewDec(int64(p)).QuoInt64Mut(int64(totalIncrements)).MulMut(inAmountDec).TruncateInt()
			}
			return inAmountIncrement
		}()
	}

	// This function computes the outAmountIncrement for a given routeIndex and inAmountIncrement.
	// It caches the result on the stack to avoid recomputing it.
	computeAndCacheOutAmount := func(routeIndex int, inAmountIncrement osmomath.Int) osmomath.Int {
		routeJOutAmountIncrement := osmomath.Int{}

		return func() osmomath.Int {
			// If the route has already been computed, return the cached value.
			// Otherwise, compute the value and cache it.
			if routeJOutAmountIncrement.IsNil() {
				// This is the expensive computation that we aim to avoid.
				curRouteOutAmountIncrement, _ := routes[routeIndex].CalculateTokenOutByTokenIn(ctx, sdk.NewCoin(tokenIn.Denom, inAmountIncrement))

				if curRouteOutAmountIncrement.IsNil() || curRouteOutAmountIncrement.IsZero() {
					curRouteOutAmountIncrement.Amount = zero
				}

				routeJOutAmountIncrement = curRouteOutAmountIncrement.Amount
			}

			return routeJOutAmountIncrement
		}()
	}

	// Step 2: fill the tables
	for x := uint8(1); x <= totalIncrements; x++ {
		for j := 1; j <= len(routes); j++ {
			dp[x][j] = dp[x][j-1] // Not using the j-th route
			proportions[x][j] = 0 // Default increment (0% of the token)

			for p := uint8(0); p <= x; p++ {
				// Consider two scenarios:
				// 1) Not using the j-th route at all, which would yield an output of dp[x][j-1].
				// 2) Using the j-th route with a certain proportion p of the input.
				//
				// The recurrence relation would be:
				// dp[x][j] = max(dp[x][j−1], dp[x−p][j−1] + output from j - th route with proportion p)
				noChoice := dp[x][j]
				choice := dp[x-p][j-1].Add(computeAndCacheOutAmount(j-1, computeAndCacheInAmountIncrement(p)))

				if choice.GT(noChoice) {
					dp[x][j] = choice
					proportions[x][j] = p
				}
			}
		}
	}

	// Step 3: trace back to find the optimal proportions
	x, j := totalIncrements, len(routes)
	optimalProportions := make([]uint8, len(routes)+1)
	for j > 0 {
		optimalProportions[j] = proportions[x][j]
		x -= proportions[x][j]
		j -= 1
	}

	optimalProportions = optimalProportions[1:]

	bestSplit := split{
		routeIncrements: optimalProportions,
		amountOut:       dp[totalIncrements][len(routes)],
	}

	tokenAmountDec := tokenIn.Amount.ToLegacyDec()

	if bestSplit.amountOut.IsZero() {
		return nil, errors.New("amount out is zero, try increasing amount in")
	}

	// Step 4: validate the found choice
	totalIncrementsInSplits := uint8(0)
	resultRoutes := make([]domain.SplitRoute, 0, len(routes))
	totalAmoutOutFromSplits := osmomath.ZeroInt()
	for i, currentRouteIncrement := range bestSplit.routeIncrements {
		currentRoute := routes[i]

		currentRouteAmtOut := computeAndCacheOutAmount(i, computeAndCacheInAmountIncrement(currentRouteIncrement))

		currentRouteSplit := sdk.NewDec(int64(currentRouteIncrement)).QuoInt64Mut(int64(totalIncrements))

		inAmount := currentRouteSplit.MulMut(tokenAmountDec).TruncateInt()
		outAmount := currentRouteAmtOut

		isAmountInNilOrZero := inAmount.IsNil() || inAmount.IsZero()
		isAmountOutNilOrZero := outAmount.IsNil() || outAmount.IsZero()
		if isAmountInNilOrZero && isAmountOutNilOrZero {
			continue
		}

		if isAmountInNilOrZero {
			return nil, fmt.Errorf("in amount is zero when out is not (%s), route index (%d)", outAmount, i)
		}

		if isAmountOutNilOrZero {
			return nil, fmt.Errorf("out amount is zero when in is not (%s), route index (%d)", inAmount, i)
		}

		resultRoutes = append(resultRoutes, &RouteWithOutAmount{
			RouteImpl: currentRoute,
			InAmount:  inAmount,
			OutAmount: currentRouteAmtOut,
		})

		totalIncrementsInSplits += currentRouteIncrement
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
