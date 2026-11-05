package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
		r := routes[0]
		coinOut, err := r.CalculateTokenOutByTokenIn(ctx, tokenIn)
		if err != nil {
			return nil, err
		}

		quote := &quoteExactAmountIn{
			AmountIn:  tokenIn,
			AmountOut: coinOut.Amount,
			Route: []domain.SplitRoute{&route.RouteWithOutAmount{
				RouteImpl: r,
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
	for i := range int(totalIncrements + 1) {
		dp[i] = make([]osmomath.Int, len(routes)+1)

		dp[i][0] = zero

		proportions[i] = make([]uint8, len(routes)+1)
	}

	// Initialize the first column with 0
	for j := 0; j <= len(routes); j++ {
		dp[0][j] = zero
	}

	inAmountDec := tokenIn.Amount.ToLegacyDec()

	// callback with caching capabilities.
	computeAndCacheOutAmountCb := getComputeAndCacheOutAmountCb(ctx, inAmountDec, tokenIn.Denom, routes)

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
				choice := dp[x-p][j-1].Add(computeAndCacheOutAmountCb(j-1, p))

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
	resultRouteIdx := make([]int, 0, len(routes))
	totalAmoutOutFromSplits := osmomath.ZeroInt()
	for i, currentRouteIncrement := range bestSplit.routeIncrements {
		currentRoute := routes[i]

		currentRouteAmtOut := computeAndCacheOutAmountCb(i, currentRouteIncrement)

		currentRouteSplit := osmomath.NewDec(int64(currentRouteIncrement)).QuoInt64Mut(int64(totalIncrements))

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

		resultRoutes = append(resultRoutes, &route.RouteWithOutAmount{
			RouteImpl: currentRoute,
			InAmount:  inAmount,
			OutAmount: currentRouteAmtOut,
		})
		resultRouteIdx = append(resultRouteIdx, i)

		totalIncrementsInSplits += currentRouteIncrement
		totalAmoutOutFromSplits = totalAmoutOutFromSplits.Add(currentRouteAmtOut)
	}

	if !totalAmoutOutFromSplits.Equal(bestSplit.amountOut) {
		return nil, fmt.Errorf("total amount out from splits (%s) does not equal actual amount out (%s)", totalAmoutOutFromSplits, bestSplit.amountOut)
	}

	// This may happen if one of the routes is consistently returning 0 amount out for all increments.
	// TODO: we may want to remove this check so that we get the best quote.
	if totalIncrementsInSplits != totalIncrements {
		return nil, fmt.Errorf("total increments (%d) does not match expected total increments (%d)", totalIncrementsInSplits, totalIncrements)
	}

	// Legs sharing a transmuter are only checked against it one at a time above; check the sum.
	if remaining, dropped, err := routesWithoutOverdrawnLegs(ctx, routes, resultRoutes, resultRouteIdx, tokenIn.Denom, true); err != nil {
		return nil, err
	} else if dropped {
		return getSplitQuote(ctx, remaining, tokenIn)
	}

	quote := &quoteExactAmountIn{
		AmountIn:  tokenIn,
		AmountOut: bestSplit.amountOut,
		Route:     resultRoutes,
	}

	return quote, nil
}

func isTransmuter(pool domain.RoutablePool) bool {
	t := pool.GetSQSType()
	return t == domain.AlloyedTransmuter || t == domain.TransmuterV1
}

// transmuterHop is the amount passing through one transmuter in one direction.
type transmuterHop struct {
	pool   domain.RoutablePool
	amount osmomath.Int
}

// transmuterHopKey groups hops. An entry with balance set has no token in denom: it groups by
// the denom withdrawn only, since legs entering a multi-asset transmuter through different
// variants draw on the same balance of the denom they withdraw.
type transmuterHopKey struct {
	poolID        uint64
	tokenInDenom  string
	tokenOutDenom string
	balance       bool
}

// legTransmuterHops walks a leg hop by hop, as the route calculation does, and returns the
// amount that enters each transmuter on it (exact-in) or leaves it (exact-out), after taker fees,
// and, under the balance key, the amount that leaves it in both directions.
// Exact-out routes list pools from token out, so the walk runs output side first. denom is the
// quote's token in (exact-in) or token out (exact-out); the walk reads each hop's denoms from the
// coin it carries, since routable pools only know their token in denom once the result is prepared.
func legTransmuterHops(ctx context.Context, leg domain.SplitRoute, denom string, exactIn bool) (map[transmuterHopKey]transmuterHop, error) {
	hops := make(map[transmuterHopKey]transmuterHop)
	pools := leg.GetPools()
	if len(pools) == 0 {
		return hops, nil
	}

	coin := sdk.NewCoin(denom, leg.GetAmountOut())
	if exactIn {
		coin = sdk.NewCoin(denom, leg.GetAmountIn())
	}

	var err error
	for _, pool := range pools {
		if exactIn {
			coin = pool.ChargeTakerFeeExactIn(coin)
		}
		hopAmount, hopDenom := coin.Amount, coin.Denom
		if exactIn {
			coin, err = pool.CalculateTokenOutByTokenIn(ctx, coin)
		} else {
			coin, err = pool.CalculateTokenInByTokenOut(ctx, coin)
		}
		if err != nil {
			return nil, err
		}
		if isTransmuter(pool) {
			id, in, out := pool.GetId(), hopDenom, coin.Denom
			if !exactIn {
				in, out = coin.Denom, hopDenom
			}
			outAmount := hopAmount
			if exactIn {
				outAmount = coin.Amount
			}
			hops[transmuterHopKey{id, in, out, false}] = transmuterHop{pool, hopAmount}
			hops[transmuterHopKey{poolID: id, tokenOutDenom: out, balance: true}] = transmuterHop{pool, outAmount}
		}
		if !exactIn {
			coin = pool.ChargeTakerFeeExactOut(coin)
		}
	}
	return hops, nil
}

// routesWithoutOverdrawnLegs checks every transmuter that more than one leg of a split passes
// through, at any hop. The DP scores each leg against the transmuter's full balance and static
// limit on its own, so the sum of the legs may be more than the pool allows. The amounts the legs
// send through the same transmuter and direction are summed and run through the pool's own
// calculation. Legs are kept largest first while the running total fits; the others are dropped.
// legRoutes[i] is the index in routes of legs[i]. It returns routes without the routes of the
// dropped legs and whether any was dropped.
func routesWithoutOverdrawnLegs(ctx context.Context, routes []route.RouteImpl, legs []domain.SplitRoute, legRoutes []int, denom string, exactIn bool) ([]route.RouteImpl, bool, error) {
	// Nothing to check unless a transmuter is on more than one leg.
	legsPerPool := make(map[uint64]int)
	shared := false
	for _, leg := range legs {
		seen := make(map[uint64]struct{})
		for _, pool := range leg.GetPools() {
			if _, ok := seen[pool.GetId()]; ok || !isTransmuter(pool) {
				continue
			}
			seen[pool.GetId()] = struct{}{}
			legsPerPool[pool.GetId()]++
			shared = shared || legsPerPool[pool.GetId()] > 1
		}
	}
	if !shared {
		return routes, false, nil
	}

	legHops := make([]map[transmuterHopKey]transmuterHop, len(legs))
	legsByKey := make(map[transmuterHopKey][]int)
	for i, leg := range legs {
		hops, err := legTransmuterHops(ctx, leg, denom, exactIn)
		if err != nil {
			return nil, false, err
		}
		legHops[i] = hops
		for key := range hops {
			legsByKey[key] = append(legsByKey[key], i)
		}
	}

	keys := make([]transmuterHopKey, 0, len(legsByKey))
	for key, sharing := range legsByKey {
		if len(sharing) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a].poolID != keys[b].poolID {
			return keys[a].poolID < keys[b].poolID
		}
		if keys[a].tokenInDenom != keys[b].tokenInDenom {
			return keys[a].tokenInDenom < keys[b].tokenInDenom
		}
		if keys[a].tokenOutDenom != keys[b].tokenOutDenom {
			return keys[a].tokenOutDenom < keys[b].tokenOutDenom
		}
		return !keys[a].balance && keys[b].balance
	})

	dropped := make(map[int]struct{})
	for _, key := range keys {
		sharing := make([]int, 0, len(legsByKey[key]))
		for _, i := range legsByKey[key] {
			if _, ok := dropped[i]; !ok {
				sharing = append(sharing, i)
			}
		}
		if len(sharing) < 2 {
			continue
		}
		sort.SliceStable(sharing, func(a, b int) bool {
			return legHops[sharing[a]][key].amount.GT(legHops[sharing[b]][key].amount)
		})

		pool := legHops[sharing[0]][key].pool
		total := osmomath.ZeroInt()
		for n, i := range sharing {
			next := total.Add(legHops[i][key].amount)
			var err error
			switch {
			case key.balance:
				// Only the balance of the withdrawn denom is checked here; the limiter is
				// checked per token in denom.
				_, err = pool.CalculateTokenInByTokenOut(ctx, sdk.NewCoin(key.tokenOutDenom, next))
				if !errors.As(err, &domain.TransmuterInsufficientBalanceError{}) {
					err = nil
				}
			case exactIn:
				_, err = pool.CalculateTokenOutByTokenIn(ctx, sdk.NewCoin(key.tokenInDenom, next))
			default:
				_, err = pool.CalculateTokenInByTokenOut(ctx, sdk.NewCoin(key.tokenOutDenom, next))
			}
			// The largest leg already passed on its own when the DP scored it.
			if err != nil && n > 0 {
				dropped[i] = struct{}{}
				continue
			}
			total = next
		}
	}

	if len(dropped) == 0 {
		return routes, false, nil
	}

	droppedRoutes := make(map[int]struct{}, len(dropped))
	for i := range dropped {
		droppedRoutes[legRoutes[i]] = struct{}{}
	}
	kept := make([]route.RouteImpl, 0, len(routes))
	for i, r := range routes {
		if _, ok := droppedRoutes[i]; !ok {
			kept = append(kept, r)
		}
	}
	return kept, true, nil
}

// unreachableInput is a sentinel marker for a dynamic-programming state that cannot be
// reached (a required amount of output cannot be produced by the routes considered so far).
// It is represented as a nil osmomath.Int so it can never be confused with a real input cost
// of zero, and any arithmetic on it is guarded by the isReachable helper below.
var unreachableInput = osmomath.Int{}

func isReachable(v osmomath.Int) bool {
	return !v.IsNil()
}

// getSplitQuoteInGivenOut returns the best exact-out (in-given-out) quote for the given
// routes and desired tokenOut. It is the input-minimising dual of getSplitQuote: where
// getSplitQuote splits the input across routes to MAXIMISE output, this splits the desired
// OUTPUT across routes to MINIMISE the total input required.
//
// The algorithm mirrors getSplitQuote's dynamic program, with four inversions:
//  1. dp[x][j] holds the MINIMUM input to produce x output-increments using the first j
//     routes (getSplitQuote holds the maximum output for x input-increments).
//  2. Unreachable states are seeded with the unreachableInput sentinel rather than zero,
//     because the minimum input to produce a positive output with no routes is undefined,
//     not zero. dp[0][*] = 0 (zero output needs zero input).
//  3. The optimiser keeps the smaller input (choice.LT(best)) rather than the larger output.
//  4. Per-increment cost is the input required for a proportion of the desired output, via
//     CalculateTokenInByTokenOut, rather than the output for a proportion of the input.
//
// The returned quote is a true exact-out quote (quoteExactAmountIn left nil).
func getSplitQuoteInGivenOut(ctx context.Context, routes []route.RouteImpl, tokenOut sdk.Coin) (domain.Quote, error) {
	// Routes must be non-empty
	if len(routes) == 0 {
		return nil, errors.New("no routes")
	}

	// If only one route, return the best single-route exact-out quote.
	if len(routes) == 1 {
		r := routes[0]
		coinIn, err := r.CalculateTokenInByTokenOut(ctx, tokenOut)
		if err != nil {
			return nil, err
		}

		quote := &quoteExactAmountOut{
			AmountIn:  coinIn.Amount,
			AmountOut: tokenOut,
			Route: []domain.SplitRoute{&route.RouteWithOutAmount{
				RouteImpl: r,
				OutAmount: tokenOut.Amount,
				InAmount:  coinIn.Amount,
			}},
		}

		return quote, nil
	}

	// proportions[x][j] records the output proportion routed through the j-th route at the
	// optimal (minimum-input) state. dp[x][j] holds that minimum input.
	proportions := make([][]uint8, totalIncrements+1)
	dp := make([][]osmomath.Int, totalIncrements+1)

	// Step 1: initialise tables.
	//   dp[0][*] = 0     -> producing zero output costs zero input.
	//   dp[x>0][0] = unreachable -> positive output cannot be produced with zero routes.
	for x := 0; x < int(totalIncrements+1); x++ {
		dp[x] = make([]osmomath.Int, len(routes)+1)
		proportions[x] = make([]uint8, len(routes)+1)

		for j := 0; j <= len(routes); j++ {
			if x == 0 {
				dp[x][j] = zero
			} else {
				dp[x][j] = unreachableInput
			}
		}
	}

	outAmountDec := tokenOut.Amount.ToLegacyDec()

	// callback with caching capabilities: input required for proportion p of the desired output
	// through a given route.
	computeAndCacheInAmountCb := getComputeAndCacheInAmountForOutCb(ctx, outAmountDec, tokenOut.Denom, routes)

	// Step 2: fill the tables.
	for x := uint8(1); x <= totalIncrements; x++ {
		for j := 1; j <= len(routes); j++ {
			// Default: do not use the j-th route; carry forward the best from the first j-1 routes.
			dp[x][j] = dp[x][j-1]
			proportions[x][j] = 0

			for p := uint8(1); p <= x; p++ {
				// Cost of producing p output-increments through route j-1.
				routeInput := computeAndCacheInAmountCb(j-1, p)
				if !isReachable(routeInput) {
					// Route cannot fill this proportion (error / insufficient liquidity).
					continue
				}

				// Remaining x-p output-increments must come from the first j-1 routes.
				remainder := dp[x-p][j-1]
				if !isReachable(remainder) {
					continue
				}

				choice := remainder.Add(routeInput)

				if !isReachable(dp[x][j]) || choice.LT(dp[x][j]) {
					dp[x][j] = choice
					proportions[x][j] = p
				}
			}
		}
	}

	// If the full desired output cannot be produced by any combination, fail rather than
	// returning a partial fill.
	if !isReachable(dp[totalIncrements][len(routes)]) {
		return nil, errors.New("desired output cannot be produced by the available routes")
	}

	// Step 3: trace back to find the optimal proportions.
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
		amountOut:       tokenOut.Amount,
	}

	// Step 4: build the result routes and validate.
	//
	// Each selected route is assigned a proportion of the desired output. Independent
	// truncation of those proportions can leave the per-route outputs summing to slightly
	// less than the requested tokenOut. For exact-out this is not acceptable: the user must
	// receive exactly tokenOut. We therefore assign any truncation remainder to the LAST
	// selected route and compute that route's input from its exact (remainder-adjusted)
	// output, so the outputs sum to tokenOut.Amount precisely.

	// First pass: collect the selected route indices and their nominal (truncated) outputs.
	type selected struct {
		routeIndex int
		increment  uint8
		outAmount  osmomath.Int
	}
	selectedRoutes := make([]selected, 0, len(routes))
	totalIncrementsInSplits := uint8(0)
	nominalOutSum := osmomath.ZeroInt()

	for i, currentRouteIncrement := range bestSplit.routeIncrements {
		if currentRouteIncrement == 0 {
			continue
		}

		currentRouteSplit := osmomath.NewDec(int64(currentRouteIncrement)).QuoInt64Mut(int64(totalIncrements))
		outAmount := currentRouteSplit.MulMut(outAmountDec).TruncateInt()

		selectedRoutes = append(selectedRoutes, selected{
			routeIndex: i,
			increment:  currentRouteIncrement,
			outAmount:  outAmount,
		})
		totalIncrementsInSplits += currentRouteIncrement
		nominalOutSum = nominalOutSum.Add(outAmount)
	}

	if totalIncrementsInSplits != totalIncrements {
		return nil, fmt.Errorf("total increments (%d) does not match expected total increments (%d)", totalIncrementsInSplits, totalIncrements)
	}
	if len(selectedRoutes) == 0 {
		return nil, errors.New("no routes selected for split")
	}

	// Assign the truncation remainder to the last selected route so the outputs sum to
	// exactly tokenOut.Amount.
	remainder := tokenOut.Amount.Sub(nominalOutSum)
	selectedRoutes[len(selectedRoutes)-1].outAmount = selectedRoutes[len(selectedRoutes)-1].outAmount.Add(remainder)

	// Second pass: compute the exact input for each route's exact output and build results.
	resultRoutes := make([]domain.SplitRoute, 0, len(selectedRoutes))
	totalAmountInFromSplits := osmomath.ZeroInt()
	totalAmountOutFromSplits := osmomath.ZeroInt()

	for _, sel := range selectedRoutes {
		currentRoute := routes[sel.routeIndex]
		outAmount := sel.outAmount

		if outAmount.IsNil() || !outAmount.IsPositive() {
			return nil, fmt.Errorf("non-positive out amount (%s) for selected route index (%d)", outAmount, sel.routeIndex)
		}

		// Compute the exact input required for this route's exact output (not the cached
		// increment estimate), charging exact-out taker fees along the route.
		coinIn, err := currentRoute.CalculateTokenInByTokenOut(ctx, sdk.NewCoin(tokenOut.Denom, outAmount))
		if err != nil {
			return nil, fmt.Errorf("computing input for split route index (%d): %w", sel.routeIndex, err)
		}
		inAmount := coinIn.Amount

		if inAmount.IsNil() || !inAmount.IsPositive() {
			return nil, fmt.Errorf("in amount is non-positive (%s) when out is %s, route index (%d)", inAmount, outAmount, sel.routeIndex)
		}

		resultRoutes = append(resultRoutes, &route.RouteWithOutAmount{
			RouteImpl: currentRoute,
			InAmount:  inAmount,
			OutAmount: outAmount,
		})

		totalAmountInFromSplits = totalAmountInFromSplits.Add(inAmount)
		totalAmountOutFromSplits = totalAmountOutFromSplits.Add(outAmount)
	}

	// The split outputs must sum to exactly the requested output.
	if !totalAmountOutFromSplits.Equal(tokenOut.Amount) {
		return nil, fmt.Errorf("total output from splits (%s) does not equal requested output (%s)", totalAmountOutFromSplits, tokenOut.Amount)
	}

	// See getSplitQuote: legs sharing a transmuter are only checked against it one at a time.
	legRoutes := make([]int, len(selectedRoutes))
	for i, sel := range selectedRoutes {
		legRoutes[i] = sel.routeIndex
	}
	if remaining, dropped, err := routesWithoutOverdrawnLegs(ctx, routes, resultRoutes, legRoutes, tokenOut.Denom, false); err != nil {
		return nil, err
	} else if dropped {
		return getSplitQuoteInGivenOut(ctx, remaining, tokenOut)
	}

	quote := &quoteExactAmountOut{
		AmountIn:  totalAmountInFromSplits,
		AmountOut: sdk.NewCoin(tokenOut.Denom, totalAmountOutFromSplits),
		Route:     resultRoutes,
	}

	return quote, nil
}

// getComputeAndCacheInAmountForOutCb returns a callback computing (and caching) the input
// required to obtain a proportion `increment`/totalIncrements of the desired total output via
// a given route, charging exact-out taker fees along the way. A route that errors (e.g.
// insufficient liquidity) yields the unreachableInput sentinel for that increment so the DP
// will not select it.
func getComputeAndCacheInAmountForOutCb(ctx context.Context, totalOutAmountDec osmomath.Dec, tokenOutDenom string, routes []route.RouteImpl) func(int, uint8) osmomath.Int {
	routeInAmtCache := make(map[int]map[uint8]osmomath.Int, len(routes))
	for routeIndex := 0; routeIndex < len(routes); routeIndex++ {
		routeInAmtCache[routeIndex] = make(map[uint8]osmomath.Int, totalIncrements+1)
	}

	computeAndCacheOutAmountIncrementCb := getComputeAndCacheOutAmountIncrementCb(totalOutAmountDec)

	return func(routeIndex int, increment uint8) osmomath.Int {
		if cached, ok := routeInAmtCache[routeIndex][increment]; ok {
			return cached
		}

		outAmountIncrement := computeAndCacheOutAmountIncrementCb(increment)

		curRouteInAmount, err := routes[routeIndex].CalculateTokenInByTokenOut(ctx, sdk.NewCoin(tokenOutDenom, outAmountIncrement))

		// If the route errors (e.g., insufficient liquidity), mark this increment unreachable
		// so the optimiser does not route output through it.
		if err != nil {
			routeInAmtCache[routeIndex][increment] = unreachableInput
			return unreachableInput
		}

		if curRouteInAmount.Amount.IsNil() || curRouteInAmount.Amount.IsZero() {
			// Zero input for positive output is not a valid fill; mark unreachable.
			routeInAmtCache[routeIndex][increment] = unreachableInput
			return unreachableInput
		}

		routeInAmtCache[routeIndex][increment] = curRouteInAmount.Amount
		return curRouteInAmount.Amount
	}
}

// getComputeAndCacheOutAmountIncrementCb computes the output amount for a given proportion p
// of the desired total output. (Mirror of getComputeAndCacheInAmountIncrementCb for the
// exact-out direction.)
func getComputeAndCacheOutAmountIncrementCb(totalOutAmountDec osmomath.Dec) func(p uint8) osmomath.Int {
	outAmountIncrements := make(map[uint8]osmomath.Int, totalIncrements)
	return func(p uint8) osmomath.Int {
		if currentIncrement, ok := outAmountIncrements[p]; ok {
			return currentIncrement
		}

		currentIncrement := osmomath.NewDec(int64(p)).QuoInt64Mut(int64(totalIncrements)).MulMut(totalOutAmountDec).TruncateInt()
		outAmountIncrements[p] = currentIncrement

		return currentIncrement
	}
}

// This function computes the inAmountIncrement for a given proportion p.
// It caches the result on the stack to avoid recomputing it.
func getComputeAndCacheInAmountIncrementCb(totalInAmountDec osmomath.Dec) func(p uint8) osmomath.Int {
	inAmountIncrements := make(map[uint8]osmomath.Int, totalIncrements)
	return func(p uint8) osmomath.Int {
		// If the inAmountIncrement has already been computed, return the cached value.
		// Otherwise, compute the value and cache it.
		currentIncrement, ok := inAmountIncrements[p]
		if ok {
			return currentIncrement
		}

		currentIncrement = osmomath.NewDec(int64(p)).QuoInt64Mut(int64(totalIncrements)).MulMut(totalInAmountDec).TruncateInt()
		inAmountIncrements[p] = currentIncrement

		return currentIncrement
	}
}

// This function computes the outAmountIncrement for a given routeIndex and inAmountIncrement.
// It caches the result on the stack to avoid recomputing it.
func getComputeAndCacheOutAmountCb(ctx context.Context, totalInAmountDec osmomath.Dec, tokenInDenom string, routes []route.RouteImpl) func(int, uint8) osmomath.Int {
	// Pre-compute routes cache map.
	routeOutAmtCache := make(map[int]map[uint8]osmomath.Int, len(routes))
	for routeIndex := range routes {
		routeOutAmtCache[routeIndex] = make(map[uint8]osmomath.Int, totalIncrements+1)
	}

	// Get callback with in amount increment capabilities.
	computeAndCacheInAmountIncrementCb := getComputeAndCacheInAmountIncrementCb(totalInAmountDec)

	return func(routeIndex int, increment uint8) osmomath.Int {
		inAmountIncrement := computeAndCacheInAmountIncrementCb(increment)

		curRouteAmt, ok := routeOutAmtCache[routeIndex][increment]
		if ok {
			return curRouteAmt
		}
		// This is the expensive computation that we aim to avoid.
		curRouteOutAmountIncrement, err := routes[routeIndex].CalculateTokenOutByTokenIn(ctx, sdk.NewCoin(tokenInDenom, inAmountIncrement))

		// If the route errors (e.g., insufficient liquidity in orderbook pools),
		// treat this route as producing zero output for this increment.
		// This ensures routes with liquidity issues are not selected by the DP algorithm.
		if err != nil {
			routeOutAmtCache[routeIndex][increment] = zero
			return zero
		}

		if curRouteOutAmountIncrement.IsNil() || curRouteOutAmountIncrement.IsZero() {
			curRouteOutAmountIncrement.Amount = zero
		}

		routeOutAmtCache[routeIndex][increment] = curRouteOutAmountIncrement.Amount

		return curRouteOutAmountIncrement.Amount
	}
}
