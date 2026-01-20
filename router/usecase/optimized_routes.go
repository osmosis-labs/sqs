package usecase

import (
	"context"
	"fmt"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"go.uber.org/zap"

	"github.com/osmosis-labs/sqs/domain"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// Returns best quote, all routes sorted by amount out, whether probe fallback was used, and error if any.
//
// When usedProbeFallback is true, the returned quote's OutAmount is an APPROXIMATION
// based on a smaller probe amount (10% of requested), not the actual output for the full InAmount.
// This happens when all routes fail at the full amount but succeed at a smaller probe amount,
// indicating the routes have limited capacity. Callers should run the split algorithm when
// usedProbeFallback is true to get accurate output calculations, rather than returning the
// quote directly.
//
// CONTRACT: router repository must be set on the router.
// CONTRACT: pools repository must be set on the router
func (r *routerUseCaseImpl) estimateAndRankSingleRouteQuoteOutGivenIn(ctx context.Context, routes route.RouteImpls, tokenIn sdk.Coin) (quote domain.Quote, sortedRoutesByAmtOut []route.RouteWithOutAmount, usedProbeFallback bool, err error) {
	if len(routes) == 0 {
		return nil, nil, false, fmt.Errorf("no routes were provided for token in (%s)", tokenIn.Denom)
	}

	// Compute token out for each route
	routesWithAmountOut, errors := routes.CalculateTokenOutByTokenIn(ctx, tokenIn)

	// If we skipped all routes due to errors, try a smaller probe amount as fallback.
	// This allows the split algorithm to handle routes with limited capacity
	// (e.g., orderbook pools that can only handle a portion of the swap).
	if len(routesWithAmountOut) == 0 && len(errors) > 0 {
		// Try with 10% of the original amount as a probe to identify viable routes
		probeAmount := tokenIn.Amount.QuoRaw(10)
		if probeAmount.IsZero() {
			probeAmount = osmomath.OneInt()
		}
		probeCoin := sdk.NewCoin(tokenIn.Denom, probeAmount)

		// Retry route calculation with smaller probe amount
		routesWithAmountOut, _ = routes.CalculateTokenOutByTokenIn(ctx, probeCoin)

		if len(routesWithAmountOut) > 0 {
			// Routes work at smaller amounts - they have limited capacity.
			// IMPORTANT: Mark that probe fallback was used. The OutAmount in these routes
			// is calculated from the probe amount, NOT the full requested amount.
			// Callers must NOT return this quote directly for single-route scenarios,
			// as it would show users an incorrect (much smaller) output amount.
			usedProbeFallback = true

			// Update InAmount to original requested amount for proper handling downstream.
			// The split algorithm will recalculate actual outputs based on each route's capacity.
			for i := range routesWithAmountOut {
				routesWithAmountOut[i].InAmount = tokenIn.Amount
			}
			// Continue to sorting below - don't return error
		} else {
			// Even probe amount failed - truly no viable routes exist.
			// Invalidate caches to force recomputation on next request.
		// This might be helpful in alloyed cases where the pool gets imbalanced and runs out of liquidity.
		// If the original routes were computed only through the zero liquidity token, they will be recomputed
		// through another token due to changed order.

		// Note: the zero length check occurred at the start of function.
		tokenOutDenom := routes[0].GetTokenOutDenom()

		r.candidateRouteCache.Delete(formatCandidateRouteCacheKey(domain.TokenSwapMethodExactIn, tokenIn.Denom, tokenOutDenom))
		tokenInOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenIn.Amount)
		r.rankedRouteCache.Delete(formatRankedRouteCacheKey(domain.TokenSwapMethodExactIn, tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude))

			return nil, nil, false, errors[0]
		}
	}

	// Sort by amount out in descending order
	sort.Slice(routesWithAmountOut, func(i, j int) bool {
		return routesWithAmountOut[i].OutAmount.GT(routesWithAmountOut[j].OutAmount)
	})

	bestRoute := routesWithAmountOut[0]

	finalQuote := &quoteExactAmountIn{
		AmountIn:  tokenIn,
		AmountOut: bestRoute.OutAmount,
		Route:     []domain.SplitRoute{&bestRoute},
	}

	return finalQuote, routesWithAmountOut, usedProbeFallback, nil
}

// Returns best quote as well as all routes sorted by amount in and error if any.
// CONTRACT: router repository must be set on the router.
// CONTRACT: pools reporitory must be set on the router
func (r *routerUseCaseImpl) estimateAndRankSingleRouteQuoteInGivenOut(ctx context.Context, routes []route.RouteImpl, tokenOut sdk.Coin, logger log.Logger) (quote domain.Quote, sortedRoutesByAmtOut []route.RouteWithOutAmount, err error) { //nolint:unused
	if len(routes) == 0 {
		return nil, nil, fmt.Errorf("no routes were provided for token in (%s)", tokenOut.Denom)
	}

	routesWithAmountIn := make([]route.RouteWithOutAmount, 0, len(routes))

	errors := []error{}

	for _, r := range routes {
		directRouteTokenIn, err := r.CalculateTokenInByTokenOut(ctx, tokenOut)
		if err != nil {
			logger.Debug("skipping single route due to error in estimate", zap.Error(err))
			errors = append(errors, err)
			continue
		}

		if directRouteTokenIn.Amount.IsNil() {
			directRouteTokenIn.Amount = osmomath.ZeroInt()
		}

		routesWithAmountIn = append(routesWithAmountIn, route.RouteWithOutAmount{
			RouteImpl: r,
			InAmount:  directRouteTokenIn.Amount,
			OutAmount: tokenOut.Amount,
		})
	}

	// If we skipped all routes due to errors, return the first error
	if len(routesWithAmountIn) == 0 && len(errors) > 0 {
		// If we encounter this problem, we attempte to invalidate all caches to recompute the routes
		// completely.
		// This might be helpful in alloyed cases where the pool gets imbalanced and runs out of liquidity.
		// If the original routes were computed only through the zero liquidity token, they will be recomputed
		// through another token due to changed order.

		// Note: the zero length check occurred at the start of function.
		tokenOutDenom := routes[0].GetTokenOutDenom()

		r.candidateRouteCache.Delete(formatCandidateRouteCacheKey(domain.TokenSwapMethodExactOut, tokenOut.Denom, tokenOutDenom))
		tokenInOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenOut.Amount)
		r.rankedRouteCache.Delete(formatRankedRouteCacheKey(domain.TokenSwapMethodExactOut, tokenOut.Denom, tokenOutDenom, tokenInOrderOfMagnitude))

		return nil, nil, errors[0]
	}

	// Sort by amount out in ascending order
	sort.Slice(routesWithAmountIn, func(i, j int) bool {
		return routesWithAmountIn[i].InAmount.LT(routesWithAmountIn[j].InAmount)
	})

	bestRoute := routesWithAmountIn[0]

	finalQuote := &quoteExactAmountOut{
		AmountIn:  bestRoute.InAmount,
		AmountOut: tokenOut,
		Route:     []domain.SplitRoute{&bestRoute},
	}

	return finalQuote, routesWithAmountIn, nil
}

// validateAndFilterRoutesOutGivenIn validates all routes. Specifically:
// - all routes have at least one pool.
// - all routes have the same final token out denom.
// - the final token out denom is not the same as the token in denom.
// - intermediary pools in the route do not contain the token in denom or token out denom.
// - the previous pool token out denom is in the current pool.
// - the current pool token out denom is in the current pool.
// Returns error if not. Nil otherwise.
func validateAndFilterRoutesOutGivenIn(candidateRoutes []candidateRouteWrapper, tokenInDenom string, logger log.Logger) (ingesttypes.CandidateRoutes, error) {
	var (
		tokenOutDenom  string
		filteredRoutes []ingesttypes.CandidateRoute
	)

	uniquePoolIDs := make(map[uint64]struct{})

	containsCanonicalOrderbook := false

ROUTE_LOOP:
	for i, candidateRoute := range candidateRoutes {
		candidateRoutePools := candidateRoute.Pools

		containsCanonicalOrderbook = containsCanonicalOrderbook || candidateRoute.IsCanonicalOrderboolRoute

		if len(candidateRoute.Pools) == 0 {
			return ingesttypes.CandidateRoutes{}, NoPoolsInRouteError{RouteIndex: i}
		}

		lastPool := candidateRoutePools[len(candidateRoutePools)-1]
		currentRouteTokenOutDenom := lastPool.TokenOutDenom

		// Validate that route pools do not have the token in denom or token out denom
		previousTokenOut := tokenInDenom

		uniquePoolIDsIntraRoute := make(map[uint64]struct{}, len(candidateRoutePools))

		for j, currentPool := range candidateRoutePools {
			if _, ok := uniquePoolIDs[currentPool.ID]; !ok {
				uniquePoolIDs[currentPool.ID] = struct{}{}
			}

			// Skip routes for which we have already seen the pool ID within that route.
			if _, ok := uniquePoolIDsIntraRoute[currentPool.ID]; ok {
				continue ROUTE_LOOP
			} else {
				uniquePoolIDsIntraRoute[currentPool.ID] = struct{}{}
			}

			currentPoolDenoms := candidateRoutePools[j].PoolDenoms
			currentPoolTokenOutDenom := currentPool.TokenOutDenom

			// Check that token in denom and token out denom are in the pool
			// Also check that previous token out is in the pool
			foundPreviousTokenOut := false
			foundCurrentTokenOut := false
			for _, denom := range currentPoolDenoms {
				if denom == previousTokenOut {
					foundPreviousTokenOut = true
				}

				if denom == currentPoolTokenOutDenom {
					foundCurrentTokenOut = true
				}

				// Validate that intermediary pools do not contain the token in denom or token out denom
				if j > 0 && j < len(candidateRoutePools)-1 {
					if denom == tokenInDenom {
						logger.Warn("route skipped - found token in intermediary pool", zap.Error(RoutePoolWithTokenInDenomError{RouteIndex: i, TokenInDenom: tokenInDenom}))
						continue ROUTE_LOOP
					}

					if denom == currentRouteTokenOutDenom {
						logger.Warn("route skipped- found token out in intermediary pool", zap.Error(RoutePoolWithTokenOutDenomError{RouteIndex: i, TokenOutDenom: currentPoolTokenOutDenom}))
						continue ROUTE_LOOP
					}
				}
			}

			// Ensure that the previous pool token out denom is in the current pool.
			if !foundPreviousTokenOut {
				return ingesttypes.CandidateRoutes{}, PreviousTokenOutDenomNotInPoolError{RouteIndex: i, PoolId: currentPool.ID, PreviousTokenOutDenom: previousTokenOut}
			}

			// Ensure that the current pool token out denom is in the current pool.
			if !foundCurrentTokenOut {
				return ingesttypes.CandidateRoutes{}, CurrentTokenOutDenomNotInPoolError{RouteIndex: i, PoolId: currentPool.ID, CurrentTokenOutDenom: currentPoolTokenOutDenom}
			}

			// Update previous token out denom
			previousTokenOut = currentPoolTokenOutDenom
		}

		if i > 0 {
			// Ensure that all routes have the same final token out denom
			if currentRouteTokenOutDenom != tokenOutDenom {
				return ingesttypes.CandidateRoutes{}, TokenOutMismatchBetweenRoutesError{TokenOutDenomRouteA: tokenOutDenom, TokenOutDenomRouteB: currentRouteTokenOutDenom}
			}
		}

		tokenOutDenom = currentRouteTokenOutDenom

		// Update filtered routes if this route passed all checks
		filteredRoute := ingesttypes.CandidateRoute{
			IsCanonicalOrderboolRoute: candidateRoute.IsCanonicalOrderboolRoute,
			Pools:                     make([]ingesttypes.CandidatePool, 0, len(candidateRoutePools)),
		}

		// Convert route to the final output format
		for _, pool := range candidateRoutePools {
			filteredRoute.Pools = append(filteredRoute.Pools, ingesttypes.CandidatePool{
				ID:            pool.ID,
				TokenOutDenom: pool.TokenOutDenom,
				TokenInDenom:  pool.TokenInDenom,
			})
		}

		filteredRoutes = append(filteredRoutes, filteredRoute)
	}

	if tokenOutDenom == tokenInDenom {
		return ingesttypes.CandidateRoutes{}, TokenOutDenomMatchesTokenInDenomError{Denom: tokenOutDenom}
	}

	return ingesttypes.CandidateRoutes{
		Routes:                     filteredRoutes,
		UniquePoolIDs:              uniquePoolIDs,
		ContainsCanonicalOrderbook: containsCanonicalOrderbook,
	}, nil
}
