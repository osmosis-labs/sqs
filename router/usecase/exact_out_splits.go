package usecase

import (
	"context"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/route"
)

// rankRoutesAtSplitShareInGivenOut returns the routes that need the least input to pay out an even split share of
// tokenOut, without routes that share a pool and cut to maxSplitRoutes. It is the exact-out counterpart of
// rankRoutesAtSplitShare. Routes that cannot pay out the share are left out.
// Ranking by the full amount alone leaves out routes that cannot fill the whole amount, although they can fill
// the part of it that a split gives them. The split drops a route from the proportions it cannot fill.
func rankRoutesAtSplitShareInGivenOut(ctx context.Context, routes []route.RouteImpl, tokenOut sdk.Coin, maxSplitRoutes int) []route.RouteImpl {
	share, ok := splitShare(tokenOut, maxSplitRoutes)
	if !ok {
		return nil
	}
	routesWithAmtIn := rankRoutesInGivenOut(ctx, routes, share)
	return cutRoutesForSplits(maxSplitRoutes, filterAndConvertDuplicatePoolIDRankedRoutes(routesWithAmtIn))
}

// rankRoutesInGivenOut returns the routes that can pay out tokenOut with a positive input, in increasing order of input.
func rankRoutesInGivenOut(ctx context.Context, routes []route.RouteImpl, tokenOut sdk.Coin) []route.RouteWithOutAmount {
	routesWithAmtIn := make([]route.RouteWithOutAmount, 0, len(routes))
	for _, r := range routes {
		in, err := r.CalculateTokenInByTokenOut(ctx, tokenOut)
		if err != nil || in.Amount.IsNil() || !in.Amount.IsPositive() {
			continue
		}
		routesWithAmtIn = append(routesWithAmtIn, route.RouteWithOutAmount{RouteImpl: r, InAmount: in.Amount, OutAmount: tokenOut.Amount})
	}
	sort.SliceStable(routesWithAmtIn, func(i, j int) bool { return routesWithAmtIn[i].InAmount.LT(routesWithAmtIn[j].InAmount) })
	return routesWithAmtIn
}

// getBestSplitQuoteInGivenOut returns the split quote with the least input of two sets of routes chosen from
// rankedRoutes: the best routes by the input for tokenOut as a whole, and the best by the input for an even split
// share. Each set is filtered for shared pools on its own. It is the exact-out counterpart of getBestSplitQuote.
// The split never fills the target partially: it fails when the routes cannot pay out all of tokenOut.
// The chain gives every leg of a split the maximum integer as its input limit, which a CosmWasm pool cannot be sent as
// funds. So a split of several legs must not start at a CosmWasm pool (the input side of an exact-out route is its
// last pool). When the best split does, it is computed again without such routes, and fails if they were needed.
func getBestSplitQuoteInGivenOut(ctx context.Context, rankedRoutes []route.RouteImpl, tokenOut sdk.Coin, maxSplitRoutes int) (domain.Quote, error) {
	best, err := getBestSplitQuoteInGivenOutAnyInput(ctx, rankedRoutes, tokenOut, maxSplitRoutes)
	if err != nil || len(best.GetRoute()) < 2 || !splitStartsAtCosmWasmPool(best) {
		return best, err
	}
	routes := make([]route.RouteImpl, 0, len(rankedRoutes))
	for _, r := range rankedRoutes {
		if !routeStartsAtCosmWasmPool(r.GetPools()) {
			routes = append(routes, r)
		}
	}
	if len(routes) == 0 {
		return nil, domain.ErrNoTokensOut
	}
	return getBestSplitQuoteInGivenOutAnyInput(ctx, routes, tokenOut, maxSplitRoutes)
}

// routeStartsAtCosmWasmPool returns true if the input side pool (the last one) of an exact-out route is a CosmWasm pool.
func routeStartsAtCosmWasmPool(pools []domain.RoutablePool) bool {
	return len(pools) > 0 && pools[len(pools)-1].GetType() == poolmanagertypes.CosmWasm
}

func splitStartsAtCosmWasmPool(quote domain.Quote) bool {
	for _, leg := range quote.GetRoute() {
		if routeStartsAtCosmWasmPool(leg.GetPools()) {
			return true
		}
	}
	return false
}

func getBestSplitQuoteInGivenOutAnyInput(ctx context.Context, rankedRoutes []route.RouteImpl, tokenOut sdk.Coin, maxSplitRoutes int) (domain.Quote, error) {
	wholeRoutes := cutRoutesForSplits(maxSplitRoutes, filterAndConvertDuplicatePoolIDRankedRoutes(rankRoutesInGivenOut(ctx, rankedRoutes, tokenOut)))
	shareRoutes := rankRoutesAtSplitShareInGivenOut(ctx, rankedRoutes, tokenOut, maxSplitRoutes)

	var (
		best    domain.Quote
		bestErr error
	)
	for _, routes := range [][]route.RouteImpl{wholeRoutes, shareRoutes} {
		if len(routes) == 0 {
			continue
		}
		quote, err := getSplitQuoteInGivenOut(ctx, routes, tokenOut)
		if err != nil {
			if bestErr == nil {
				bestErr = err
			}
			continue
		}
		if best == nil || quote.GetAmountIn().Amount.LT(best.GetAmountIn().Amount) {
			best = quote
		}
	}
	if best != nil {
		return best, nil
	}
	if bestErr == nil {
		bestErr = domain.ErrNoTokensOut
	}
	return nil, bestErr
}
