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

	type splitPrecompute struct {
		amountIn  osmomath.Int
		amountOut osmomath.Int
	}

	// Compute in amount
	inAmountIncrements := make(map[uint8]osmomath.Int, totalIncrements)
	inAmountIncrements[0] = osmomath.ZeroInt()
	inAmountIncrements[totalIncrements-1] = tokenIn.Amount

	inAmountDec := tokenIn.Amount.ToLegacyDec()

	outAmountPrecompute := make([][]osmomath.Int, len(routes)+1)
	for i, currentRoute := range routes {
		outAmountPrecompute[i] = make([]osmomath.Int, totalIncrements+1)

		outAmountPrecompute[i][0] = osmomath.ZeroInt()

		for j := uint8(1); j <= totalIncrements; j++ {

			inAmountIncrement := sdk.NewDec(int64(j)).QuoInt64Mut(int64(totalIncrements)).MulMut(inAmountDec).TruncateInt()

			curRouteOutAmountIncrement, _ := currentRoute.CalculateTokenOutByTokenIn(ctx, sdk.NewCoin(tokenIn.Denom, inAmountIncrement))

			if curRouteOutAmountIncrement.IsNil() || curRouteOutAmountIncrement.IsZero() {
				curRouteOutAmountIncrement.Amount = zero
			}

			outAmountPrecompute[i][j] = curRouteOutAmountIncrement.Amount
		}
	}

	proportions := make([][]uint8, totalIncrements+1)
	dp := make([][]osmomath.Int, totalIncrements+1)
	for i := 0; i < int(totalIncrements+1); i++ {
		dp[i] = make([]osmomath.Int, len(routes)+1)

		dp[i][0] = osmomath.ZeroInt()

		proportions[i] = make([]uint8, len(routes)+1)
	}

	for i := 0; i <= len(routes); i++ {
		dp[0][i] = osmomath.ZeroInt()
	}

	for x := uint8(1); x <= totalIncrements; x++ {
		for j := 1; j <= len(routes); j++ {
			dp[x][j] = dp[x][j-1] // Not using the j-th route
			proportions[x][j] = 0 // Default increment (0% of the token)

			for p := uint8(0); p <= x; p++ {
				noChoice := dp[x][j]
				choice := dp[x-p][j-1].Add(outAmountPrecompute[j-1][p])

				if choice.GT(noChoice) {
					dp[x][j] = choice
					proportions[x][j] = p
				}
			}
		}
	}

	// Trace back to find the optimal proportions
	x, j := totalIncrements, len(routes)
	optimalProportions := make([]uint8, len(routes)+1)
	for j > 0 {
		optimalProportions[j] = proportions[x][j]
		x -= uint8(proportions[x][j]) // Convert proportion back to index
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

	totalIncrementsInSplits := uint8(0)
	resultRoutes := make([]domain.SplitRoute, 0, len(routes))
	totalAmoutOutFromSplits := osmomath.ZeroInt()
	for i, currentRouteIncrement := range bestSplit.routeIncrements {
		currentRoute := routes[i]

		currentRouteIndex := uint8(i)

		if currentRouteIncrement < 0 {
			return nil, fmt.Errorf("best increment for route %d is negative", currentRouteIndex)
		}

		currentRouteAmtOut := outAmountPrecompute[currentRouteIndex][currentRouteIncrement]

		currentRouteSplit := sdk.NewDec(int64(currentRouteIncrement)).QuoInt64Mut(int64(totalIncrements))

		inAmount := currentRouteSplit.MulMut(tokenAmountDec).TruncateInt()
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
