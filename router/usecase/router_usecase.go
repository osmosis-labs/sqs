package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/osmoutils"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	routertypes "github.com/osmosis-labs/sqs/router/types"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"
)

var _ mvc.RouterUsecase = &routerUseCaseImpl{}

type routerUseCaseImpl struct {
	routerRepository       mvc.RouterRepository
	poolsUsecase           mvc.PoolsUsecase
	tokenMetadataHolder    mvc.TokenMetadataHolder
	candidateRouteSearcher domain.CandidateRouteSearcher

	// This is the default config used when no routing options are provided.
	defaultConfig       domain.RouterConfig
	cosmWasmPoolsConfig domain.CosmWasmPoolRouterConfig
	logger              log.Logger

	rankedRouteCache *cache.Cache

	sortedPoolsMu sync.RWMutex
	sortedPools   []ingesttypes.PoolI

	candidateRouteCache *cache.Cache
}

const (
	candidateRouteCacheLabel = "candidate_route"
	rankedRouteCacheLabel    = "ranked_route"

	denomSeparatorChar = "|"
)

var zero = osmomath.ZeroInt()

// NewRouterUsecase will create a new pools use case object
func NewRouterUsecase(
	routerRepository mvc.RouterRepository,
	poolsUsecase mvc.PoolsUsecase,
	candidateRouteSearcher domain.CandidateRouteSearcher,
	tokenMetadataHolder mvc.TokenMetadataHolder,
	config domain.RouterConfig,
	cosmWasmPoolsConfig domain.CosmWasmPoolRouterConfig,
	logger log.Logger,
	rankedRouteCache *cache.Cache,
	candidateRouteCache *cache.Cache,
) mvc.RouterUsecase {
	return &routerUseCaseImpl{
		routerRepository:       routerRepository,
		poolsUsecase:           poolsUsecase,
		tokenMetadataHolder:    tokenMetadataHolder,
		defaultConfig:          config,
		cosmWasmPoolsConfig:    cosmWasmPoolsConfig,
		candidateRouteSearcher: candidateRouteSearcher,
		logger:                 logger,

		rankedRouteCache:    rankedRouteCache,
		candidateRouteCache: candidateRouteCache,

		sortedPools:   make([]ingesttypes.PoolI, 0),
		sortedPoolsMu: sync.RWMutex{},
	}
}

// GetOptimalQuoteOutGivenIn returns the optimal quote by estimating the optimal route(s) through pools
// on the osmosis network.
// Uses default router config if no options parameter is provided.
// With the options parameter, you can customize the router behavior. See domain.RouterOption for more details.
// Uses caching strategies for optimal performance.
// Currently, supports candidate route caching. If candidate routes for the given token in and token out denoms
// are present in cache, they are used without re-computing them. Otherwise, they are computed and cached.
// In the future, we will support caching of ranked routes that are constructed from candidate and sorted
// by the decreasing amount out within an order of magnitude of token in. Similarly, We will also support optimal split caching
// Returns error if:
// - fails to estimate direct quotes for ranked routes
// - fails to retrieve candidate routes
func (r *routerUseCaseImpl) GetOptimalQuoteOutGivenIn(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
	options := domain.RouterOptions{
		MaxPoolsPerRoute:                 r.defaultConfig.MaxPoolsPerRoute,
		MaxRoutes:                        r.defaultConfig.MaxRoutes,
		MinPoolLiquidityCap:              r.defaultConfig.MinPoolLiquidityCap,
		CandidateRouteCacheExpirySeconds: r.defaultConfig.CandidateRouteCacheExpirySeconds,
		RankedRouteCacheExpirySeconds:    r.defaultConfig.RankedRouteCacheExpirySeconds,
		MaxSplitRoutes:                   r.defaultConfig.MaxSplitRoutes,
		DisableCache:                     !r.defaultConfig.RouteCacheEnabled,
		CandidateRoutesPoolFiltersAnyOf:  []domain.CandidateRoutePoolFiltrerCb{},
	}
	// Apply options
	for _, opt := range opts {
		opt(&options)
	}

	var (
		candidateRankedRoutes ingesttypes.CandidateRoutes
		err                   error
	)

	if !options.DisableCache {
		// Get an order of magnitude for the token in amount
		// This is used for caching ranked routes as these might differ depending on the amount swapped in.
		tokenInOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenIn.Amount)

		candidateRankedRoutes, err = r.GetCachedRankedRoutes(ctx, domain.TokenSwapMethodExactIn, tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude, options.MaxSplitRoutes)
		if err != nil {
			return nil, err
		}
	}

	var (
		topSingleRouteQuote domain.Quote
		rankedRoutes        []route.RouteImpl
	)

	// If no cached candidate routes are found, we attempt to
	// compute them.
	if len(candidateRankedRoutes.Routes) == 0 {
		options.MinPoolLiquidityCap = r.getQuoteMinPoolLiquidityCapFilter(tokenIn.Denom, tokenOutDenom, options.MinPoolLiquidityCap)

		// Find candidate routes and rank them by direct quotes.
		topSingleRouteQuote, rankedRoutes, err = r.computeAndRankRoutesByDirectQuoteOutGivenIn(ctx, tokenIn, tokenOutDenom, options)
		if err != nil {
			return nil, err
		}
	} else {
		// Otherwise, simply compute quotes over cached ranked routes.
		// If every route fails, invalidate the entries this quote reads: the ranked routes and the candidate routes
		// that a ranked route cache miss would search with.
		onAllRoutesFailed := func() {
			r.invalidateExactInRouteCaches(tokenIn, tokenOutDenom, options.MaxSplitRoutes)
		}
		topSingleRouteQuote, rankedRoutes, err = r.rankRoutesByDirectQuoteOutGivenIn(ctx, candidateRankedRoutes, tokenIn, tokenOutDenom, options.MaxSplitRoutes, onAllRoutesFailed)
		if err != nil {
			return nil, err
		}
	}

	if len(rankedRoutes) == 1 || options.MaxSplitRoutes == domain.DisableSplitRoutes {
		return validateQuoteAmountOut(topSingleRouteQuote)
	}

	// Filter out generalized cosmWasm pool routes
	rankedRoutes = filterOutGeneralizedCosmWasmPoolRoutes(rankedRoutes)

	// If filtering leads to a single route left, return it.
	if len(rankedRoutes) == 1 {
		return validateQuoteAmountOut(topSingleRouteQuote)
	}

	// Compute split route quote
	topSplitQuote, err := getSplitQuote(ctx, rankedRoutes, tokenIn)
	if err != nil {
		// If error occurs in splits, return the single route quote
		// rather than failing.
		return validateQuoteAmountOut(topSingleRouteQuote)
	}

	finalQuote := topSingleRouteQuote

	// If the split route quote is better than the single route quote, return the split route quote
	if topSplitQuote.GetAmountOut().Amount.GT(topSingleRouteQuote.GetAmountOut().Amount) {
		routes := topSplitQuote.GetRoute()

		r.logger.Debug("split route selected", zap.Int("route_count", len(routes)))

		finalQuote = topSplitQuote
	}

	r.logger.Debug("single route selected", zap.Stringer("route", finalQuote.GetRoute()[0]))

	return validateQuoteAmountOut(finalQuote)
}

// validateQuoteAmountOut returns an error if the quote yields no tokens out,
// for example when a dust amount in is fully consumed by fees and rounding.
func validateQuoteAmountOut(quote domain.Quote) (domain.Quote, error) {
	amountOut := quote.GetAmountOut().Amount
	if amountOut.IsNil() || amountOut.IsZero() {
		return nil, domain.ErrNoTokensOut
	}

	return quote, nil
}

// GetOptimalQuoteInGivenOut returns an optimal quote through the pools for the exact amount out token swap method.
// Note: the parameter names from the interface are misleading — tokenIn is the desired output coin and
// tokenOutDenom is the denom the caller will pay. We use clear aliases below to avoid confusion.
func (r *routerUseCaseImpl) GetOptimalQuoteInGivenOut(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
	// Clarify argument semantics: the caller specifies how much they want to receive (desiredOut)
	// and which token they will pay (inputDenom).
	desiredOut := tokenIn
	inputDenom := tokenOutDenom

	options := domain.RouterOptions{
		MaxPoolsPerRoute:                 r.defaultConfig.MaxPoolsPerRoute,
		MaxRoutes:                        r.defaultConfig.MaxRoutes,
		MinPoolLiquidityCap:              r.defaultConfig.MinPoolLiquidityCap,
		CandidateRouteCacheExpirySeconds: r.defaultConfig.CandidateRouteCacheExpirySeconds,
		RankedRouteCacheExpirySeconds:    r.defaultConfig.RankedRouteCacheExpirySeconds,
		MaxSplitRoutes:                   r.defaultConfig.MaxSplitRoutes,
		DisableCache:                     !r.defaultConfig.RouteCacheEnabled,
		CandidateRoutesPoolFiltersAnyOf: []domain.CandidateRoutePoolFiltrerCb{
			// Orderbook contract does not implement MsgSwapExactAmountOut.
			domain.ShouldSkipOrderbookPool,
		},
	}
	for _, opt := range opts {
		opt(&options)
	}

	// Get the dynamic min pool liquidity cap for the given token pair.
	dynamicMinPoolLiquidityCap, err := r.tokenMetadataHolder.GetMinPoolEffectiveLiquidityCap(desiredOut.Denom, inputDenom)
	if err == nil {
		options.MinPoolLiquidityCap = r.ConvertMinTokensPoolLiquidityCapToFilter(dynamicMinPoolLiquidityCap)
	} else {
		dynamicMinPoolLiquidityCap, err := r.tokenMetadataHolder.GetMinPoolLiquidityCap(desiredOut.Denom, inputDenom)
		if err == nil {
			options.MinPoolLiquidityCap = r.ConvertMinTokensPoolLiquidityCapToFilter(dynamicMinPoolLiquidityCap)
		}
	}

	topSingleRouteQuote, rankedRoutes, err := r.computeAndRankRoutesByDirectQuoteInGivenOut(ctx, desiredOut, inputDenom, options)
	if err != nil {
		return nil, err
	}

	// Single route, or splitting disabled: return the single best route.
	if len(rankedRoutes) == 1 || options.MaxSplitRoutes == domain.DisableSplitRoutes {
		return topSingleRouteQuote, nil
	}

	// Filter out generalized cosmWasm pool routes (consistent with the out-given-in path).
	rankedRoutes = filterOutGeneralizedCosmWasmPoolRoutes(rankedRoutes)

	// If filtering leaves a single route, return it.
	if len(rankedRoutes) == 1 {
		return topSingleRouteQuote, nil
	}

	// Compute the split-route quote that minimises the required input for the desired output.
	topSplitQuote, err := getSplitQuoteInGivenOut(ctx, rankedRoutes, desiredOut)
	if err != nil {
		// If the split computation fails, fall back to the single best route rather than
		// failing the quote.
		return topSingleRouteQuote, nil
	}

	finalQuote := topSingleRouteQuote

	// Exact-out selection is by MINIMUM input: prefer the split quote only when it requires
	// strictly less input than the single best route for the same desired output.
	if topSplitQuote.GetAmountIn().Amount.LT(topSingleRouteQuote.GetAmountIn().Amount) {
		r.logger.Debug("split route selected", zap.Int("route_count", len(topSplitQuote.GetRoute())))
		finalQuote = topSplitQuote
	}

	if finalQuote.GetAmountIn().Amount.IsZero() {
		return nil, errors.New("best we can do requires zero input")
	}

	return finalQuote, nil
}

// GetSimpleQuote implements mvc.RouterUsecase.
// TODO: cover with a simple test.
func (r *routerUseCaseImpl) GetSimpleQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, []domain.SplitRoute, error) {
	options := domain.RouterOptions{
		MaxPoolsPerRoute:    r.defaultConfig.MaxPoolsPerRoute,
		MaxRoutes:           r.defaultConfig.MaxRoutes,
		MinPoolLiquidityCap: r.defaultConfig.MinPoolLiquidityCap,
		MaxSplitRoutes:      r.defaultConfig.MaxSplitRoutes,
	}
	// Apply options
	for _, opt := range opts {
		opt(&options)
	}

	dynamicMinPoolLiquidityCap, err := r.tokenMetadataHolder.GetMinPoolLiquidityCap(tokenIn.Denom, tokenOutDenom)
	if err == nil {
		// Set the dynamic min pool liquidity cap only if there is no error retrieving it.
		// Oterwise, use default.
		// The dynamic filter may only raise the floor passed in the options, never lower it,
		// so that a configured pricing minimum keeps dust pools from setting prices.
		options.MinPoolLiquidityCap = max(options.MinPoolLiquidityCap, r.ConvertMinTokensPoolLiquidityCapToFilter(dynamicMinPoolLiquidityCap))
	}

	// If this is pricing worker precomputation, we need to be able to call this as
	// some pools have TVL incorrectly calculated as zero. For example, BRNCH / STRDST (1288).
	// As a result, they are incorrectly excluded despite having appropriate liquidity.
	// So we want to calculate price, but we never cache routes for pricing the are below the minPoolLiquidityCap value, as these are returned to users.

	// Compute candidate routes.
	candidateRouteSearchOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           options.MaxRoutes,
		MaxPoolsPerRoute:    options.MaxPoolsPerRoute,
		MinPoolLiquidityCap: options.MinPoolLiquidityCap,
		PoolFiltersAnyOf:    options.CandidateRoutesPoolFiltersAnyOf,
	}

	candidateRoutes, err := r.candidateRouteSearcher.FindCandidateRoutesOutGivenIn(ctx, tokenIn, tokenOutDenom, candidateRouteSearchOptions)
	if err != nil {
		r.logger.Error("error getting candidate routes for pricing", zap.Error(err))
		return nil, nil, err
	}

	routesImpl, err := r.poolsUsecase.GetRoutesFromCandidates(candidateRoutes, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		r.logger.Error("error ranking routes for pricing", zap.Error(err))
		return nil, nil, err
	}

	// The simple quote neither reads nor writes the route caches, so there is nothing to invalidate.
	quote, routes, err := r.estimateAndRankSingleRouteQuoteOutGivenIn(ctx, routesImpl, tokenIn, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("%s, tokenOutDenom (%s)", err, tokenOutDenom)
	}

	var splitRoutes []domain.SplitRoute
	for i := range routes {
		splitRoutes = append(splitRoutes, &routes[i])
	}

	return quote, splitRoutes, nil
}

// filterAndConvertDuplicatePoolIDRankedRoutes filters ranked routes that contain duplicate pool IDs.
// Routes with overlapping Alloyed and transmuter pools are not filtered out.
// Additionally, the routes are converted into route.Route.Impl type.
// CONTRACT: rankedRoutes are sorted in decreasing order by amount out
// from first to last.
func filterAndConvertDuplicatePoolIDRankedRoutes(rankedRoutes []route.RouteWithOutAmount) []route.RouteImpl {
	// We use two maps for all routes and for the current route.
	// This is so that if a route ends up getting filtered, its pool IDs are not added to the combined map.
	combinedPoolIDsMap := make(map[uint64]struct{})
	filteredRankedRoutes := make([]route.RouteImpl, 0)

	for _, route := range rankedRoutes {
		pools := route.GetPools()

		currentRoutePoolIDsMap := make(map[uint64]struct{})

		existsPoolID := false

		for _, pool := range pools {
			// Skip transmuter pools since they offer no slippage benefits.
			if pool.GetSQSType() == domain.AlloyedTransmuter || pool.GetSQSType() == domain.TransmuterV1 {
				continue
			}

			poolID := pool.GetId()

			_, existsPoolID = combinedPoolIDsMap[poolID]

			// If found a pool ID that was already seen, break and skip the route.
			if existsPoolID {
				break
			}

			// Add pool ID to current pool IDs map
			currentRoutePoolIDsMap[poolID] = struct{}{}
		}

		// If pool ID exists, we skip this route
		if existsPoolID {
			continue
		}

		// Merge current route pool IDs map into the combined map
		for poolID := range currentRoutePoolIDsMap {
			combinedPoolIDsMap[poolID] = struct{}{}
		}

		// Add route to filtered ranked routes
		filteredRankedRoutes = append(filteredRankedRoutes, route.RouteImpl)
	}
	return filteredRankedRoutes
}

// rankRoutesByDirectQuoteOutGivenIn ranks the given candidate routes by estimating direct quotes over each route.
// Additionally, it filters out routes with duplicate pool IDs and cuts them for splits
// based on the value of maxSplitRoutes.
// Returns the top quote as well as the ranked routes in decrease order of amount out.
// Returns error if:
// - fails to read taker fees
// - fails to convert candidate routes to routes
// - fails to estimate direct quotes
// onAllRoutesFailed, if not nil, is called when every route fails to estimate a direct quote.
func (r *routerUseCaseImpl) rankRoutesByDirectQuoteOutGivenIn(ctx context.Context, candidateRoutes ingesttypes.CandidateRoutes, tokenIn sdk.Coin, tokenOutDenom string, maxSplitRoutes int, onAllRoutesFailed func()) (domain.Quote, []route.RouteImpl, error) {
	// Note that retrieving pools and taker fees is done in separate transactions.
	// This is fine because taker fees don't change often.
	routes, err := r.poolsUsecase.GetRoutesFromCandidates(candidateRoutes, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, nil, err
	}

	topQuote, routesWithAmtOut, err := r.estimateAndRankSingleRouteQuoteOutGivenIn(ctx, routes, tokenIn, onAllRoutesFailed)
	if err != nil {
		return nil, nil, fmt.Errorf("%s, tokenOutDenom (%s)", err, tokenOutDenom)
	}

	// Update ranked routes with filtered ranked routes
	routes = filterAndConvertDuplicatePoolIDRankedRoutes(routesWithAmtOut)

	// Cut routes for splits
	routes = cutRoutesForSplits(maxSplitRoutes, routes)

	return topQuote, routes, nil
}

// rankRoutesByDirectQuoteInGivenOut ranks the given candidate routes by estimating direct quotes over each route.
// Additionally, it filters out routes with duplicate pool IDs and cuts them for splits
// based on the value of maxSplitRoutes.
// Returns the top quote as well as the ranked routes in decrease order of amount out.
// Returns error if:
// - fails to read taker fees
// - fails to convert candidate routes to routes
// - fails to estimate direct quotes
// onAllRoutesFailed, if not nil, is called when every route fails to estimate a direct quote.
func (r *routerUseCaseImpl) rankRoutesByDirectQuoteInGivenOut(ctx context.Context, candidateRoutes ingesttypes.CandidateRoutes, tokenOut sdk.Coin, tokenInDenom string, maxSplitRoutes int, onAllRoutesFailed func()) (domain.Quote, []route.RouteImpl, error) {
	// Note that retrieving pools and taker fees is done in separate transactions.
	// This is fine because taker fees don't change often.
	routes, err := r.poolsUsecase.GetRoutesFromCandidatesInGivenOut(candidateRoutes)
	if err != nil {
		return nil, nil, err
	}

	topQuote, routesWithAmtOut, err := r.estimateAndRankSingleRouteQuoteInGivenOut(ctx, routes, tokenOut, r.logger, onAllRoutesFailed)
	if err != nil {
		return nil, nil, fmt.Errorf("%s, tokenOutDenom (%s)", err, tokenInDenom)
	}

	// Update ranked routes with filtered ranked routes
	routes = filterAndConvertDuplicatePoolIDRankedRoutes(routesWithAmtOut)

	// Cut routes for splits
	routes = cutRoutesForSplits(maxSplitRoutes, routes)

	return topQuote, routes, nil
}

// computeAndRankRoutesByDirectQuoteOutGivenIn computes candidate routes and ranks them by token out after estimating direct quotes.
func (r *routerUseCaseImpl) computeAndRankRoutesByDirectQuoteOutGivenIn(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, routingOptions domain.RouterOptions) (domain.Quote, []route.RouteImpl, error) {
	tokenInOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenIn.Amount)

	candidateRouteSearchOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           routingOptions.MaxRoutes,
		MaxPoolsPerRoute:    routingOptions.MaxPoolsPerRoute,
		MinPoolLiquidityCap: routingOptions.MinPoolLiquidityCap,
		DisableCache:        routingOptions.DisableCache,
		PoolFiltersAnyOf:    routingOptions.CandidateRoutesPoolFiltersAnyOf,
	}

	// If top routes are not present in cache, retrieve unranked candidate routes
	candidateRoutes, err := r.handleCandidateRoutesOutGivenIn(ctx, tokenIn, tokenOutDenom, candidateRouteSearchOptions)
	if err != nil {
		r.logger.Error("error handling routes", zap.Error(err))
		return nil, nil, err
	}

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	// The candidate route cache is owned by handleCandidateRoutes, which only writes it on a miss.
	// Re-setting it here would refresh its expiry on every ranked route cache miss, so that
	// the candidate routes of an active pair would never be recomputed.
	if !routingOptions.DisableCache && len(candidateRoutes.Routes) == 0 {
		r.rankedRouteCache.Set(formatRankedRouteCacheKey(domain.TokenSwapMethodExactIn, tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude, routingOptions.MaxSplitRoutes), candidateRoutes, time.Duration(routingOptions.RankedRouteCacheExpirySeconds/4)*time.Second)

		return nil, nil, fmt.Errorf("no candidate routes found")
	}

	// If every route fails, invalidate the entries this quote used.
	// With the cache disabled, the quote used no cache entries.
	var onAllRoutesFailed func()
	if !routingOptions.DisableCache {
		onAllRoutesFailed = func() {
			r.invalidateExactInRouteCaches(tokenIn, tokenOutDenom, routingOptions.MaxSplitRoutes)
		}
	}

	// Rank candidate routes by estimating direct quotes
	topSingleRouteQuote, rankedRoutes, err := r.rankRoutesByDirectQuoteOutGivenIn(ctx, candidateRoutes, tokenIn, tokenOutDenom, routingOptions.MaxSplitRoutes, onAllRoutesFailed)
	if err != nil {
		r.logger.Error("error getting ranked routes", zap.Error(err))
		return nil, nil, err
	}

	if len(rankedRoutes) == 0 {
		return nil, nil, fmt.Errorf("no ranked routes found")
	}

	// Convert ranked routes back to candidate for caching
	convertedCandidateRoutes := convertRankedToCandidateRoutes(rankedRoutes)

	if len(rankedRoutes) > 0 {
		// We would like to always consider the canonical orderbook route so that if new limits appear
		// we can detect them. Oterwise, our cache would have to expire to detect them.
		if !convertedCandidateRoutes.ContainsCanonicalOrderbook && candidateRoutes.ContainsCanonicalOrderbook {
			// Find the canonical orderbook route and add it to the converted candidate routes.
			for _, candidateRoute := range candidateRoutes.Routes {
				if candidateRoute.IsCanonicalOrderboolRoute {
					convertedCandidateRoutes.Routes = append(convertedCandidateRoutes.Routes, candidateRoute)
					break
				}
			}
		}

		if !routingOptions.DisableCache {
			domain.SQSRoutesCacheWritesCounter.WithLabelValues(requestURLPath, rankedRouteCacheLabel).Inc()
			r.rankedRouteCache.Set(formatRankedRouteCacheKey(domain.TokenSwapMethodExactIn, tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude, routingOptions.MaxSplitRoutes), convertedCandidateRoutes, time.Duration(routingOptions.RankedRouteCacheExpirySeconds)*time.Second)
		}
	}

	return topSingleRouteQuote, rankedRoutes, nil
}

func (r *routerUseCaseImpl) computeAndRankRoutesByDirectQuoteInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom string, routingOptions domain.RouterOptions) (domain.Quote, []route.RouteImpl, error) {
	tokenInOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenOut.Amount)

	candidateRouteSearchOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           routingOptions.MaxRoutes,
		MaxPoolsPerRoute:    routingOptions.MaxPoolsPerRoute,
		MinPoolLiquidityCap: routingOptions.MinPoolLiquidityCap,
		DisableCache:        routingOptions.DisableCache,
		PoolFiltersAnyOf:    routingOptions.CandidateRoutesPoolFiltersAnyOf,
	}

	// If top routes are not present in cache, retrieve unranked candidate
	candidateRoutes, err := r.handleCandidateRoutesInGivenOut(ctx, tokenOut, tokenInDenom, candidateRouteSearchOptions)
	if err != nil {
		r.logger.Error("error handling routes", zap.Error(err))
		return nil, nil, err
	}

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	// The candidate route cache is owned by handleCandidateRoutesInGivenOut, which only writes it on a miss.
	// Re-setting it here would refresh its expiry on every quote, so that the candidate routes of an
	// active pair would never be recomputed.
	if !routingOptions.DisableCache && len(candidateRoutes.Routes) == 0 {
		r.rankedRouteCache.Set(formatRankedRouteCacheKey(domain.TokenSwapMethodExactOut, tokenOut.Denom, tokenInDenom, tokenInOrderOfMagnitude, routingOptions.MaxSplitRoutes), candidateRoutes, time.Duration(routingOptions.RankedRouteCacheExpirySeconds/4)*time.Second)

		return nil, nil, fmt.Errorf("no candidate routes found")
	}

	// If every route fails, invalidate the entries this quote used.
	// With the cache disabled, the quote used no cache entries.
	var onAllRoutesFailed func()
	if !routingOptions.DisableCache {
		onAllRoutesFailed = func() {
			r.invalidateExactOutRouteCaches(tokenOut, tokenInDenom, routingOptions.MaxSplitRoutes)
		}
	}

	// Rank candidate routes by estimating direct quotes
	topSingleRouteQuote, rankedRoutes, err := r.rankRoutesByDirectQuoteInGivenOut(ctx, candidateRoutes, tokenOut, tokenInDenom, routingOptions.MaxSplitRoutes, onAllRoutesFailed)
	if err != nil {
		r.logger.Error("error getting ranked routes", zap.Error(err))
		return nil, nil, err
	}

	if len(rankedRoutes) == 0 {
		return nil, nil, fmt.Errorf("no ranked routes found")
	}

	// Convert ranked routes back to candidate for caching
	convertedCandidateRoutes := convertRankedToCandidateRoutes(rankedRoutes)

	if len(rankedRoutes) > 0 {
		// We would like to always consider the canonical orderbook route so that if new limits appear
		// we can detect them. Oterwise, our cache would have to expire to detect them.
		if !convertedCandidateRoutes.ContainsCanonicalOrderbook && candidateRoutes.ContainsCanonicalOrderbook {
			// Find the canonical orderbook route and add it to the converted candidate routes.
			for _, candidateRoute := range candidateRoutes.Routes {
				if candidateRoute.IsCanonicalOrderboolRoute {
					convertedCandidateRoutes.Routes = append(convertedCandidateRoutes.Routes, candidateRoute)
					break
				}
			}
		}

		if !routingOptions.DisableCache {
			domain.SQSRoutesCacheWritesCounter.WithLabelValues(requestURLPath, rankedRouteCacheLabel).Inc()
			r.rankedRouteCache.Set(formatRankedRouteCacheKey(domain.TokenSwapMethodExactOut, tokenOut.Denom, tokenInDenom, tokenInOrderOfMagnitude, routingOptions.MaxSplitRoutes), convertedCandidateRoutes, time.Duration(routingOptions.RankedRouteCacheExpirySeconds)*time.Second)
		}
	}

	return topSingleRouteQuote, rankedRoutes, nil
}

var (
	ErrTokenInDenomPoolNotFound  = fmt.Errorf("token in denom not found in pool")
	ErrTokenOutDenomPoolNotFound = fmt.Errorf("token out denom not found in pool")
)

// GetCustomDirectQuoteOutGivenIn implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCustomDirectQuoteOutGivenIn(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolID uint64) (domain.Quote, error) {
	// A pinned pool bypasses the candidate route search, so check the exclusion list here too,
	// before the lookup so an excluded pool that no longer exists still gets this error.
	if osmoutils.Contains(r.defaultConfig.ExcludedPoolIDs, poolID) {
		return nil, fmt.Errorf("%w: pool %d is excluded from routing", routertypes.ErrValidationFailed, poolID)
	}

	pool, err := r.poolsUsecase.GetPool(poolID)
	if err != nil {
		return nil, err
	}

	poolDenoms := pool.GetPoolDenoms()

	if !osmoutils.Contains(poolDenoms, tokenIn.Denom) {
		return nil, fmt.Errorf("denom %s in pool %d: %w", tokenIn.Denom, poolID, ErrTokenInDenomPoolNotFound)
	}
	if !osmoutils.Contains(poolDenoms, tokenOutDenom) {
		return nil, fmt.Errorf("denom %s in pool %d: %w", tokenOutDenom, poolID, ErrTokenOutDenomPoolNotFound)
	}

	// create candidate routes with given token out denom and pool ID.
	candidateRoutes := r.createCandidateRouteByPoolID(tokenIn.Denom, tokenOutDenom, poolID)

	// Convert candidate route into a route with all the pool data
	routes, err := r.poolsUsecase.GetRoutesFromCandidates(candidateRoutes, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, err
	}

	// Compute direct quote
	// The custom direct quote neither reads nor writes the route caches, so there is nothing to invalidate.
	bestSingleRouteQuote, _, err := r.estimateAndRankSingleRouteQuoteOutGivenIn(ctx, routes, tokenIn, nil)
	if err != nil {
		return nil, err
	}

	return validateQuoteAmountOut(bestSingleRouteQuote)
}

// GetCustomDirectQuoteOutGivenIn implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCustomDirectQuoteInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom string, poolID uint64) (domain.Quote, error) {
	pool, err := r.poolsUsecase.GetPool(poolID)
	if err != nil {
		return nil, err
	}

	poolDenoms := pool.GetPoolDenoms()

	if !osmoutils.Contains(poolDenoms, tokenOut.Denom) {
		return nil, fmt.Errorf("denom %s in pool %d: %w", tokenOut.Denom, poolID, ErrTokenInDenomPoolNotFound)
	}
	if !osmoutils.Contains(poolDenoms, tokenInDenom) {
		return nil, fmt.Errorf("denom %s in pool %d: %w", tokenInDenom, poolID, ErrTokenOutDenomPoolNotFound)
	}

	// create candidate routes with given token out denom and pool ID.
	candidateRoutes := r.createCandidateRouteByPoolID(tokenInDenom, tokenOut.Denom, poolID)

	// Convert candidate route into a route with all the pool data
	routes, err := r.poolsUsecase.GetRoutesFromCandidatesInGivenOut(candidateRoutes)
	if err != nil {
		return nil, err
	}

	// Compute direct quote
	bestSingleRouteQuote, _, err := r.estimateAndRankSingleRouteQuoteInGivenOut(ctx, routes, tokenOut, r.logger, nil)
	if err != nil {
		return nil, err
	}

	return bestSingleRouteQuote, nil
}

// GetCustomDirectQuoteMultiPoolOutGivenIn implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCustomDirectQuoteMultiPoolOutGivenIn(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom []string, poolIDs []uint64) (domain.Quote, error) {
	if len(poolIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one pool ID should be specified", routertypes.ErrValidationFailed)
	}

	if len(tokenOutDenom) == 0 {
		return nil, fmt.Errorf("%w: at least one token out denom should be specified", routertypes.ErrValidationFailed)
	}

	// for each given pool we expect to have provided token out denom
	if len(poolIDs) != len(tokenOutDenom) {
		return nil, fmt.Errorf("%w: number of pool ID should match number of out denom", routertypes.ErrValidationFailed)
	}

	// AmountIn is the first token of the asset pair.
	result := quoteExactAmountIn{AmountIn: tokenIn}

	pools := make([]domain.RoutablePool, 0, len(poolIDs))

	for i, v := range poolIDs {
		tokenOutDenom := tokenOutDenom[i]

		quote, err := r.GetCustomDirectQuoteOutGivenIn(ctx, tokenIn, tokenOutDenom, v)
		if err != nil {
			return nil, err
		}

		route := quote.GetRoute()
		if len(route) != 1 {
			return nil, fmt.Errorf("custom direct quote must have 1 route, had: %d", len(route))
		}

		poolsInRoute := route[0].GetPools()
		if len(poolsInRoute) != 1 {
			return nil, fmt.Errorf("custom direct quote route must have 1 pool, had: %d", len(poolsInRoute))
		}

		// the amountOut value is the amount out of last the tokenOutDenom
		result.AmountOut = quote.GetAmountOut().Amount

		// append each pool to the route
		pools = append(pools, poolsInRoute...)

		tokenIn = sdk.NewCoin(tokenOutDenom, quote.GetAmountOut().Amount)
	}

	// Construct the final multi-hop custom direct quote route.
	result.Route = []domain.SplitRoute{
		&route.RouteWithOutAmount{
			Pools:     pools,
			OutAmount: result.AmountOut,
			InAmount:  result.AmountIn.Amount,
		},
	}

	return &result, nil
}

// GetCustomDirectQuoteMultiPoolInGivenOut implements mvc.RouterUsecase.
//
// The pools are given from the output side: poolIDs[0] returns tokenOut in exchange for
// tokenInDenom[0], poolIDs[1] returns tokenInDenom[0] in exchange for tokenInDenom[1], and so
// on, so tokenInDenom[len-1] is what the user pays. The route is quoted with the in-given-out
// estimator, so each hop is charged its own directional taker fee and the input includes it.
func (r *routerUseCaseImpl) GetCustomDirectQuoteMultiPoolInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom []string, poolIDs []uint64) (domain.Quote, error) {
	if len(poolIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one pool ID should be specified", routertypes.ErrValidationFailed)
	}

	if len(tokenInDenom) == 0 {
		return nil, fmt.Errorf("%w: at least one token in denom should be specified", routertypes.ErrValidationFailed)
	}

	// for each given pool we expect to have provided token in denom
	if len(poolIDs) != len(tokenInDenom) {
		return nil, fmt.Errorf("%w: number of pool ID should match number of in denom", routertypes.ErrValidationFailed)
	}

	candidatePools := make([]ingesttypes.CandidatePool, 0, len(poolIDs))
	uniquePoolIDs := make(map[uint64]struct{}, len(poolIDs))

	// hopOutDenom is what the current pool must return: tokenOut for the first pool,
	// then the previous pool's input.
	hopOutDenom := tokenOut.Denom
	for i, poolID := range poolIDs {
		pool, err := r.poolsUsecase.GetPool(poolID)
		if err != nil {
			return nil, err
		}

		// The orderbook contract has no exact-out swap, so a quote through it could never execute.
		if cosmWasmPoolModel := pool.GetSQSPoolModel().CosmWasmPoolModel; cosmWasmPoolModel != nil && cosmWasmPoolModel.IsOrderbook() {
			return nil, fmt.Errorf("%w: pool %d is an orderbook, and orderbook pools do not support exact-out swaps", routertypes.ErrValidationFailed, poolID)
		}

		// The error names follow GetCustomDirectQuoteInGivenOut, which reports the
		// returned denom as the "token in" of the inverted search.
		poolDenoms := pool.GetPoolDenoms()
		if !osmoutils.Contains(poolDenoms, hopOutDenom) {
			return nil, fmt.Errorf("denom %s in pool %d: %w", hopOutDenom, poolID, ErrTokenInDenomPoolNotFound)
		}
		if !osmoutils.Contains(poolDenoms, tokenInDenom[i]) {
			return nil, fmt.Errorf("denom %s in pool %d: %w", tokenInDenom[i], poolID, ErrTokenOutDenomPoolNotFound)
		}

		candidatePools = append(candidatePools, ingesttypes.CandidatePool{
			ID:            poolID,
			TokenInDenom:  tokenInDenom[i],
			TokenOutDenom: hopOutDenom,
		})
		uniquePoolIDs[poolID] = struct{}{}

		hopOutDenom = tokenInDenom[i]
	}

	candidateRoutes := ingesttypes.CandidateRoutes{
		Routes:        []ingesttypes.CandidateRoute{{Pools: candidatePools}},
		UniquePoolIDs: uniquePoolIDs,
	}

	// Convert candidate route into a route with all the pool data
	routes, err := r.poolsUsecase.GetRoutesFromCandidatesInGivenOut(candidateRoutes)
	if err != nil {
		return nil, err
	}

	// Compute direct quote
	quote, _, err := r.estimateAndRankSingleRouteQuoteInGivenOut(ctx, routes, tokenOut, r.logger, nil)
	if err != nil {
		return nil, err
	}

	return quote, nil
}

// GetCandidateRoutes implements domain.RouterUsecase.
func (r *routerUseCaseImpl) GetCandidateRoutes(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (ingesttypes.CandidateRoutes, error) {
	candidateRouteSearchOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           r.defaultConfig.MaxRoutes,
		MaxPoolsPerRoute:    r.defaultConfig.MaxPoolsPerRoute,
		MinPoolLiquidityCap: r.defaultConfig.MinPoolLiquidityCap,
	}

	// Get the dynamic min pool liquidity cap for the given token in and token out denoms.
	dynamicMinPoolLiquidityCap, err := r.tokenMetadataHolder.GetMinPoolLiquidityCap(tokenIn.Denom, tokenOutDenom)
	if err == nil {
		// Set the dynamic min pool liquidity cap only if there is no error retrieving it.
		// Otherwise, use the default.
		candidateRouteSearchOptions.MinPoolLiquidityCap = r.ConvertMinTokensPoolLiquidityCapToFilter(dynamicMinPoolLiquidityCap)
	}

	// The routes are not for a quote of a given size, so only the shared reference-sized search is used.
	candidateRoutes, err := r.handleCandidateRoutesOutGivenIn(ctx, r.getCandidateSearchCoin(tokenIn), tokenOutDenom, candidateRouteSearchOptions)
	if err != nil {
		return ingesttypes.CandidateRoutes{}, err
	}

	return candidateRoutes, nil
}

// GetTakerFee implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetTakerFee(poolID uint64) ([]ingesttypes.TakerFeeForPair, error) {
	pool, err := r.poolsUsecase.GetPool(poolID)
	if err != nil {
		return []ingesttypes.TakerFeeForPair{}, err
	}

	poolDenoms := pool.GetPoolDenoms()

	result := make([]ingesttypes.TakerFeeForPair, 0)

	for i := range poolDenoms {
		for j := i + 1; j < len(poolDenoms); j++ {
			denom0 := poolDenoms[i]
			denom1 := poolDenoms[j]

			takerFee, ok := r.routerRepository.GetTakerFee(denom0, denom1)
			if !ok {
				return []ingesttypes.TakerFeeForPair{}, fmt.Errorf("taker fee not found for pool %d, denom in (%s), denom out (%s)", poolID, denom0, denom1)
			}

			result = append(result, ingesttypes.TakerFeeForPair{
				Denom0:   denom0,
				Denom1:   denom1,
				TakerFee: takerFee,
			})
		}
	}

	return result, nil
}

// GetCachedCandidateRoutes implements mvc.RouterUsecase.
// It returns the candidate routes of the search sized by the reference amount, which every quote on the pair uses.
func (r *routerUseCaseImpl) GetCachedCandidateRoutes(ctx context.Context, method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string) (ingesttypes.CandidateRoutes, bool, error) {
	return r.getCachedCandidateRoutesByKey(ctx, formatCandidateRouteCacheKey(method, tokenInDenom, tokenOutDenom))
}

// getCachedCandidateRoutesByKey returns the candidate routes cached under cacheKey.
// Returns false if no routes are cached under it, since zero routes may be cached.
func (r *routerUseCaseImpl) getCachedCandidateRoutesByKey(ctx context.Context, cacheKey string) (ingesttypes.CandidateRoutes, bool, error) {
	if !r.defaultConfig.RouteCacheEnabled {
		return ingesttypes.CandidateRoutes{}, false, nil
	}

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return ingesttypes.CandidateRoutes{}, false, err
	}

	cachedCandidateRoutes, found := r.candidateRouteCache.Get(cacheKey)
	if !found {
		// Increase cache misses
		domain.SQSRoutesCacheMissesCounter.WithLabelValues(requestURLPath, candidateRouteCacheLabel).Inc()

		return ingesttypes.CandidateRoutes{
			Routes:        []ingesttypes.CandidateRoute{},
			UniquePoolIDs: map[uint64]struct{}{},
		}, false, nil
	}

	domain.SQSRoutesCacheHitsCounter.WithLabelValues(requestURLPath, candidateRouteCacheLabel).Inc()

	candidateRoutes, ok := cachedCandidateRoutes.(ingesttypes.CandidateRoutes)
	if !ok {
		return ingesttypes.CandidateRoutes{}, false, fmt.Errorf("error casting candidate routes from cache")
	}

	return candidateRoutes, true, nil
}

// GetCachedRankedRoutes implements mvc.RouterUsecase.
// Ranked routes are cut to maxSplitRoutes before caching, so they are cached per maxSplitRoutes.
func (r *routerUseCaseImpl) GetCachedRankedRoutes(ctx context.Context, method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string, tokenInOrderOfMagnitude int, maxSplitRoutes int) (ingesttypes.CandidateRoutes, error) {
	if !r.defaultConfig.RouteCacheEnabled {
		return ingesttypes.CandidateRoutes{}, nil
	}

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return ingesttypes.CandidateRoutes{}, err
	}

	cachedRankedRoutes, found := r.rankedRouteCache.Get(formatRankedRouteCacheKey(method, tokenInDenom, tokenOutDenom, tokenInOrderOfMagnitude, maxSplitRoutes))
	if !found {
		// Increase cache misses
		domain.SQSRoutesCacheMissesCounter.WithLabelValues(requestURLPath, rankedRouteCacheLabel).Inc()

		return ingesttypes.CandidateRoutes{}, nil
	}

	domain.SQSRoutesCacheHitsCounter.WithLabelValues(requestURLPath, rankedRouteCacheLabel).Inc()

	rankedRoutes, ok := cachedRankedRoutes.(ingesttypes.CandidateRoutes)
	if !ok {
		return ingesttypes.CandidateRoutes{}, fmt.Errorf("error casting candidate routes from cache")
	}

	return rankedRoutes, nil
}

// handleCandidateRoutesOutGivenIn returns the exact in candidate routes for tokenIn and tokenOutDenom: the routes of a
// search sized by the reference amount of the token in (see getCandidateSearchCoin) and, for a quote of any other size,
// the routes of a search sized by the quote.
// The candidate route search skips first pools holding less of the token in than the amount it is given. A search sized
// by the reference amount keeps deep pools that hold less of the token in than a large quote, and a search sized by the
// quote leaves out small pools that would otherwise take the limited candidate route slots, or, for a quote smaller than
// the reference amount, keeps small pools that it can use. Each set is cached, the first
// per pair and the second per pair and order of magnitude (sized by the first quote of that order of magnitude), and the
// quote ranks both together.
// Errors if retrieving routes from cache or computing them fails.
func (r *routerUseCaseImpl) handleCandidateRoutesOutGivenIn(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, candidateRouteSearchOptions domain.CandidateRouteSearchOptions) (ingesttypes.CandidateRoutes, error) {
	search := func(searchCoin sdk.Coin) func() (ingesttypes.CandidateRoutes, error) {
		return func() (ingesttypes.CandidateRoutes, error) {
			return r.candidateRouteSearcher.FindCandidateRoutesOutGivenIn(ctx, searchCoin, tokenOutDenom, candidateRouteSearchOptions)
		}
	}

	return r.handleCandidateRoutes(ctx, domain.TokenSwapMethodExactIn, tokenIn, tokenOutDenom, candidateRouteSearchOptions.DisableCache, search)
}

// handleCandidateRoutesInGivenOut returns the exact out candidate routes for tokenOut and tokenInDenom, searched the same
// way as handleCandidateRoutesOutGivenIn with the token out in place of the token in.
// Errors if retrieving routes from cache or computing them fails.
func (r *routerUseCaseImpl) handleCandidateRoutesInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom string, candidateRouteSearchOptions domain.CandidateRouteSearchOptions) (ingesttypes.CandidateRoutes, error) {
	search := func(searchCoin sdk.Coin) func() (ingesttypes.CandidateRoutes, error) {
		return func() (ingesttypes.CandidateRoutes, error) {
			return r.candidateRouteSearcher.FindCandidateRoutesInGivenOut(ctx, searchCoin, tokenInDenom, candidateRouteSearchOptions)
		}
	}

	return r.handleCandidateRoutes(ctx, domain.TokenSwapMethodExactOut, tokenOut, tokenInDenom, candidateRouteSearchOptions.DisableCache, search)
}

// handleCandidateRoutes returns the union of the candidate routes of a search sized by the reference amount of coin and,
// if coin is any other amount, of a search sized by coin, cached per order of magnitude of coin.
// search returns the function that runs the candidate route search for the given coin.
func (r *routerUseCaseImpl) handleCandidateRoutes(ctx context.Context, method domain.TokenSwapMethod, coin sdk.Coin, otherDenom string, disableCache bool, search func(sdk.Coin) func() (ingesttypes.CandidateRoutes, error)) (ingesttypes.CandidateRoutes, error) {
	referenceCoin := r.getCandidateSearchCoin(coin)

	candidateRoutes, err := r.getOrSearchCandidateRoutes(ctx, formatCandidateRouteCacheKey(method, coin.Denom, otherDenom), disableCache, search(referenceCoin))
	if err != nil {
		return ingesttypes.CandidateRoutes{}, err
	}

	if coin.Amount.IsNil() || !coin.Amount.IsPositive() || coin.Amount.Equal(referenceCoin.Amount) {
		return candidateRoutes, nil
	}

	sizedCandidateRoutes, err := r.getOrSearchCandidateRoutes(ctx, formatSizedCandidateRouteCacheKey(method, coin.Denom, otherDenom, GetPrecomputeOrderOfMagnitude(coin.Amount)), disableCache, search(coin))
	if err != nil {
		return ingesttypes.CandidateRoutes{}, err
	}

	mergedCandidateRoutes, _ := mergeCandidateRoutes(sizedCandidateRoutes, candidateRoutes)

	return mergedCandidateRoutes, nil
}

// getOrSearchCandidateRoutes returns the candidate routes cached under cacheKey, or runs search and caches its result
// on a miss. With the cache disabled, it always runs search and caches nothing.
// Errors if reading the cache or the search fails.
func (r *routerUseCaseImpl) getOrSearchCandidateRoutes(ctx context.Context, cacheKey string, disableCache bool, search func() (ingesttypes.CandidateRoutes, error)) (ingesttypes.CandidateRoutes, error) {
	if !disableCache {
		candidateRoutes, isFoundCached, err := r.getCachedCandidateRoutesByKey(ctx, cacheKey)
		if err != nil {
			return ingesttypes.CandidateRoutes{}, err
		}
		if isFoundCached {
			r.logger.Debug("cached routes", zap.Int("num_routes", len(candidateRoutes.Routes)))
			return candidateRoutes, nil
		}
	}

	r.logger.Debug("calculating routes")

	start := time.Now()
	candidateRoutes, err := search()
	if err != nil {
		r.logger.Error("error getting candidate routes", zap.Error(err))
		return ingesttypes.CandidateRoutes{}, err
	}

	r.logger.Info("calculated routes", zap.Int("num_routes", len(candidateRoutes.Routes)), zap.Duration("duration", time.Since(start)))

	domain.SQSCandidateRoutesComputeDurationGauge.Set(float64(time.Since(start).Milliseconds()))

	if !disableCache {
		cacheDurationSeconds := r.defaultConfig.CandidateRouteCacheExpirySeconds
		if len(candidateRoutes.Routes) == 0 {
			// If there are no routes, we want to cache the result for a shorter duration
			// Add 1 to ensure that it is never 0 as zero signifies never clearing.
			cacheDurationSeconds = cacheDurationSeconds/4 + 1
		}

		if len(candidateRoutes.Routes) > 0 {
			requestURLPath, err := domain.GetURLPathFromContext(ctx)
			if err != nil {
				return ingesttypes.CandidateRoutes{}, err
			}
			domain.SQSRoutesCacheWritesCounter.WithLabelValues(requestURLPath, candidateRouteCacheLabel).Inc()
		}

		r.logger.Debug("persisting routes", zap.Int("num_routes", len(candidateRoutes.Routes)))
		r.candidateRouteCache.Set(cacheKey, candidateRoutes, time.Duration(cacheDurationSeconds)*time.Second)
	}

	return candidateRoutes, nil
}

// mergeCandidateRoutes returns the routes of candidateRoutes followed by the routes of extraCandidateRoutes that are
// not already in it. Returns false if extraCandidateRoutes adds no route.
func mergeCandidateRoutes(candidateRoutes, extraCandidateRoutes ingesttypes.CandidateRoutes) (ingesttypes.CandidateRoutes, bool) {
	merged := ingesttypes.CandidateRoutes{
		Routes:                     append(make([]ingesttypes.CandidateRoute, 0, len(candidateRoutes.Routes)+len(extraCandidateRoutes.Routes)), candidateRoutes.Routes...),
		UniquePoolIDs:              make(map[uint64]struct{}, len(candidateRoutes.UniquePoolIDs)+len(extraCandidateRoutes.UniquePoolIDs)),
		ContainsCanonicalOrderbook: candidateRoutes.ContainsCanonicalOrderbook,
	}
	for poolID := range candidateRoutes.UniquePoolIDs {
		merged.UniquePoolIDs[poolID] = struct{}{}
	}

	isMerged := false
	for _, extraRoute := range extraCandidateRoutes.Routes {
		if slices.ContainsFunc(merged.Routes, func(candidateRoute ingesttypes.CandidateRoute) bool {
			return slices.Equal(candidateRoute.Pools, extraRoute.Pools)
		}) {
			continue
		}

		merged.Routes = append(merged.Routes, extraRoute)
		merged.ContainsCanonicalOrderbook = merged.ContainsCanonicalOrderbook || extraRoute.IsCanonicalOrderboolRoute
		for _, pool := range extraRoute.Pools {
			merged.UniquePoolIDs[pool.ID] = struct{}{}
		}
		isMerged = true
	}

	return merged, isMerged
}

// StoreRouterStateFiles implements domain.RouterUsecase.
// TODO: clean up
func (r *routerUseCaseImpl) StoreRouterStateFiles() error {
	routerState, err := r.GetRouterState()
	if err != nil {
		return err
	}

	if err := parsing.StorePools(routerState.Pools, routerState.TickMap, "pools.json"); err != nil {
		return err
	}

	if err := parsing.StoreTakerFees("taker_fees.json", routerState.TakerFees); err != nil {
		return err
	}

	// Store candidate route search data.
	if err := parsing.StoreCandidateRouteSearchData(routerState.CandidateRouteSearchData, "candidate_route_search_data.json"); err != nil {
		return err
	}

	return nil
}

func (r *routerUseCaseImpl) LoadRouterStateFiles() error {
	pools, _, err := parsing.ReadPools("./state/pools.json")
	if err != nil {
		return err
	}

	if err := r.poolsUsecase.StorePools(pools); err != nil {
		return err
	}

	takerFeeMap, err := parsing.ReadTakerFees("./state/taker_fees.json")
	if err != nil {
		return err
	}

	r.SetTakerFees(takerFeeMap)

	candidateRouteSearchData, err := parsing.ReadCandidateRouteSearchData("./state/candidate_route_search_data.json")
	if err != nil {
		return err
	}

	r.routerRepository.SetCandidateRouteSearchData(candidateRouteSearchData)

	return nil
}

// GetRouterStateJSON implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetRouterState() (domain.RouterState, error) {
	// These pools do not contain tick model
	pools, err := r.poolsUsecase.GetAllPools()
	if err != nil {
		return domain.RouterState{}, err
	}

	concentratedpoolIDs := make([]uint64, 0, len(pools))
	for _, pool := range pools {
		if pool.GetType() == poolmanagertypes.Concentrated {
			concentratedpoolIDs = append(concentratedpoolIDs, pool.GetId())
		}
	}

	tickModelMap, err := r.poolsUsecase.GetTickModelMap(concentratedpoolIDs)
	if err != nil {
		return domain.RouterState{}, err
	}

	takerFeesMap := r.routerRepository.GetAllTakerFees()

	candidateRouteSearchData, err := r.routerRepository.GetCandidateRouteSearchData()
	if err != nil {
		return domain.RouterState{}, err
	}

	return domain.RouterState{
		Pools:                    pools,
		TakerFees:                takerFeesMap,
		TickMap:                  tickModelMap,
		CandidateRouteSearchData: candidateRouteSearchData,
	}, nil
}

// formatRouteCacheKey formats the given token in and token out denoms to a string.
func formatRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string) string {
	return fmt.Sprintf("%d%s%s%s", method, tokenInDenom, denomSeparatorChar, tokenOutDenom)
}

// formatRankedRouteCacheKey formats the given token in and token out denoms, order of magnitude and max split routes to a string.
// Max split routes is part of the key because ranked routes are cut to it before caching.
func formatRankedRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string, tokenIOrderOfMagnitude int, maxSplitRoutes int) string {
	return fmt.Sprintf("%s%s%d%s%d", formatRouteCacheKey(method, tokenInDenom, tokenOutDenom), denomSeparatorChar, tokenIOrderOfMagnitude, denomSeparatorChar, maxSplitRoutes)
}

// formatCandidateRouteCacheKey formats the given token in and token out denoms to a string.
func formatCandidateRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string) string {
	return fmt.Sprintf("cr%s", formatRouteCacheKey(method, tokenInDenom, tokenOutDenom))
}

// formatSizedCandidateRouteCacheKey formats the key of the candidate routes searched for the given order of magnitude
// of the amount.
func formatSizedCandidateRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string, orderOfMagnitude int) string {
	return fmt.Sprintf("%s%s%d", formatCandidateRouteCacheKey(method, tokenInDenom, tokenOutDenom), denomSeparatorChar, orderOfMagnitude)
}

// candidateSearchReferenceValue is the value, in units of liquidity capitalization, of the amount that sizes the
// candidate route search. It matches the amount of the quote denom that the pricing worker quotes.
const candidateSearchReferenceValue = 10

// getCandidateSearchCoin returns the coin that sizes the candidate route search for the given coin: the amount of
// its denom worth candidateSearchReferenceValue, priced from the denom's total liquidity and its capitalization.
// The search skips first pools holding less of the denom than this amount, so it drops dust pools without
// depending on the size of the quote. Returns an amount of one if the denom has no priced liquidity, so that
// only first pools holding none of the denom are skipped.
func (r *routerUseCaseImpl) getCandidateSearchCoin(coin sdk.Coin) sdk.Coin {
	poolDenomMetadata, err := r.tokenMetadataHolder.GetPoolDenomMetadata(coin.Denom)
	if err != nil || poolDenomMetadata.TotalLiquidity.IsNil() || poolDenomMetadata.TotalLiquidityCap.IsNil() ||
		!poolDenomMetadata.TotalLiquidity.IsPositive() || !poolDenomMetadata.TotalLiquidityCap.IsPositive() {
		return sdk.NewCoin(coin.Denom, osmomath.OneInt())
	}

	referenceAmount := poolDenomMetadata.TotalLiquidity.MulRaw(candidateSearchReferenceValue).Quo(poolDenomMetadata.TotalLiquidityCap)
	if !referenceAmount.IsPositive() {
		referenceAmount = osmomath.OneInt()
	}

	return sdk.NewCoin(coin.Denom, referenceAmount)
}

// convertRankedToCandidateRoutes converts the given ranked routes to candidate routes.
// The primary use case for this is to keep minimal data for caching.
func convertRankedToCandidateRoutes(rankedRoutes []route.RouteImpl) ingesttypes.CandidateRoutes {
	candidateRoutes := ingesttypes.CandidateRoutes{
		Routes:                     make([]ingesttypes.CandidateRoute, 0, len(rankedRoutes)),
		UniquePoolIDs:              map[uint64]struct{}{},
		ContainsCanonicalOrderbook: false,
	}

	for _, rankedRoute := range rankedRoutes {
		candidateRoute := ingesttypes.CandidateRoute{
			Pools:                     make([]ingesttypes.CandidatePool, 0, len(rankedRoute.GetPools())),
			IsCanonicalOrderboolRoute: rankedRoute.HasCanonicalOrderbookPool,
		}

		candidateRoutes.ContainsCanonicalOrderbook = candidateRoutes.ContainsCanonicalOrderbook || rankedRoute.HasCanonicalOrderbookPool

		for _, randkedPool := range rankedRoute.GetPools() {
			candidatePool := ingesttypes.CandidatePool{
				ID:            randkedPool.GetId(),
				TokenOutDenom: randkedPool.GetTokenOutDenom(),
			}

			candidateRoute.Pools = append(candidateRoute.Pools, candidatePool)

			candidateRoutes.UniquePoolIDs[randkedPool.GetId()] = struct{}{}
		}

		candidateRoutes.Routes = append(candidateRoutes.Routes, candidateRoute)
	}
	return candidateRoutes
}

// cutRoutesForSplits cuts the routes for splits based on the max split routes.
// If max split routes is set to DisableSplitRoutes, it will return the top route.
// If the number of routes is greater than the max split routes, it will keep only the top routes.
// If the number of routes is less than or equal to the max split routes, it will return all the routes.
func cutRoutesForSplits(maxSplitRoutes int, routes []route.RouteImpl) []route.RouteImpl {
	// If split routes are disabled, return a single the top route
	if maxSplitRoutes == domain.DisableSplitRoutes && len(routes) > 0 {
		// If there are more routes than the max split routes, keep only the top routes
		routes = routes[:1]
	} else if len(routes) > maxSplitRoutes {
		// Keep only top routes for splits
		routes = routes[:maxSplitRoutes]
	}

	return routes
}

// ConvertMinTokensPoolLiquidityCapToFilter implements mvc.RouterUsecase.
// CONTRACT: r.defaultConfig.DynamicMinLiquidityCapFiltersDesc are sorted in descending order by MinTokensCap.
func (r *routerUseCaseImpl) ConvertMinTokensPoolLiquidityCapToFilter(minTokensPoolLiquidityCap uint64) uint64 {
	for _, filter := range r.defaultConfig.DynamicMinLiquidityCapFiltersDesc {
		if minTokensPoolLiquidityCap >= filter.MinTokensCap {
			return filter.FilterValue
		}
	}
	return r.defaultConfig.MinPoolLiquidityCap
}

// getQuoteMinPoolLiquidityCapFilter returns the min pool liquidity cap filter that an exact in quote uses
// for the candidate route search between the given denoms.
// It is derived from the min pool effective liquidity cap, falling back to the min pool liquidity cap.
// If neither is available, defaultFilter is returned.
func (r *routerUseCaseImpl) getQuoteMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom string, defaultFilter uint64) uint64 {
	// Get the dynamic min pool liquidity cap for the given token in and token out denoms.
	dynamicMinPoolLiquidityCap, err := r.tokenMetadataHolder.GetMinPoolEffectiveLiquidityCap(tokenInDenom, tokenOutDenom)
	if err == nil {
		// Set the dynamic min pool liquidity cap only if there is no error retrieving it.
		// Otherwise, use the default.
		return r.ConvertMinTokensPoolLiquidityCapToFilter(dynamicMinPoolLiquidityCap)
	}

	// Fallback
	// Get the dynamic min pool liquidity cap for the given token in and token out denoms.
	dynamicMinPoolLiquidityCap, err = r.tokenMetadataHolder.GetMinPoolLiquidityCap(tokenInDenom, tokenOutDenom)
	if err == nil {
		return r.ConvertMinTokensPoolLiquidityCapToFilter(dynamicMinPoolLiquidityCap)
	}

	return defaultFilter
}

// invalidateExactInRouteCaches deletes the exact in candidate and ranked route cache entries
// that a quote for tokenIn with the given max split routes uses: both candidate route entries and the ranked entry.
// This helps in alloyed cases where the pool gets imbalanced and runs out of liquidity, so that
// the routes are recomputed completely.
func (r *routerUseCaseImpl) invalidateExactInRouteCaches(tokenIn sdk.Coin, tokenOutDenom string, maxSplitRoutes int) {
	tokenInOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenIn.Amount)

	r.candidateRouteCache.Delete(formatCandidateRouteCacheKey(domain.TokenSwapMethodExactIn, tokenIn.Denom, tokenOutDenom))
	r.candidateRouteCache.Delete(formatSizedCandidateRouteCacheKey(domain.TokenSwapMethodExactIn, tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude))
	r.rankedRouteCache.Delete(formatRankedRouteCacheKey(domain.TokenSwapMethodExactIn, tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude, maxSplitRoutes))
}

// invalidateExactOutRouteCaches deletes the exact out candidate and ranked route cache entries that a quote for
// tokenOut and tokenInDenom read or wrote: both candidate route entries, and the ranked entry for the order of
// magnitude of tokenOut and max split routes.
func (r *routerUseCaseImpl) invalidateExactOutRouteCaches(tokenOut sdk.Coin, tokenInDenom string, maxSplitRoutes int) {
	tokenOutOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenOut.Amount)

	r.candidateRouteCache.Delete(formatCandidateRouteCacheKey(domain.TokenSwapMethodExactOut, tokenOut.Denom, tokenInDenom))
	r.candidateRouteCache.Delete(formatSizedCandidateRouteCacheKey(domain.TokenSwapMethodExactOut, tokenOut.Denom, tokenInDenom, tokenOutOrderOfMagnitude))
	r.rankedRouteCache.Delete(formatRankedRouteCacheKey(domain.TokenSwapMethodExactOut, tokenOut.Denom, tokenInDenom, tokenOutOrderOfMagnitude, maxSplitRoutes))
}

// getMinPoolLiquidityCapFilter returns the min liquidity cap filter for the given tokenIn and tokenOutDenom.
// If the mapping between min liquidity cap and the filter is not found, it will return the default per config.
// Returns the min liquidity cap filter and an error if any.
func (r *routerUseCaseImpl) GetMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom string) (uint64, error) {
	defaultMinLiquidityCap := r.defaultConfig.MinPoolLiquidityCap

	minPoolLiquidityCapBetweenTokens, err := r.tokenMetadataHolder.GetMinPoolLiquidityCap(tokenInDenom, tokenOutDenom)
	if err != nil {
		// If fallback is enabled, get defaiult config value as fallback
		return defaultMinLiquidityCap, nil
	}

	// Otherwise, use the mapping to convert from min pool liquidity cap between token in and out denoms
	// to the proposed filter.
	minPoolLiquidityCapFilter := r.ConvertMinTokensPoolLiquidityCapToFilter(minPoolLiquidityCapBetweenTokens)

	return minPoolLiquidityCapFilter, nil
}

// GetPoolSpotPrice implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetPoolSpotPrice(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error) {
	poolTakerFee, ok := r.routerRepository.GetTakerFee(quoteAsset, baseAsset)
	if !ok {
		return osmomath.BigDec{}, fmt.Errorf("taker fee not found for pool %d, denom in (%s), denom out (%s)", poolID, quoteAsset, baseAsset)
	}

	spotPrice, err := r.poolsUsecase.GetPoolSpotPrice(ctx, poolID, poolTakerFee, quoteAsset, baseAsset)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return spotPrice, nil
}

// GetBaseFee implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetBaseFee() domain.BaseFee {
	return r.routerRepository.GetBaseFee()
}

// SetSortedPools implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) SetSortedPools(pools []ingesttypes.PoolI) {
	r.sortedPoolsMu.Lock()
	r.sortedPools = pools
	r.sortedPoolsMu.Unlock()
}

// SetTakerFees implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) SetTakerFees(takerFees ingesttypes.TakerFeeMap) {
	r.routerRepository.SetTakerFees(takerFees)
}

// GetSortedPools implements mvc.RouterUsecase.
// Note that this method is not thread safe.
func (r *routerUseCaseImpl) GetSortedPools() []ingesttypes.PoolI {
	return r.sortedPools
}

// GetConfig implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetConfig() domain.RouterConfig {
	return r.defaultConfig
}

// filterOutGeneralizedCosmWasmPoolRoutes filters out routes that contain generalized cosm wasm pool.
// The reason for this is that making network requests to chain is expensive. Generalized cosmwasm pools
// make such network requests.
// As a result, we want to minimize the number of requests we make by excluding such routes from split quotes.
func filterOutGeneralizedCosmWasmPoolRoutes(rankedRoutes []route.RouteImpl) []route.RouteImpl {
	result := make([]route.RouteImpl, 0)
	for _, route := range rankedRoutes {
		if route.ContainsGeneralizedCosmWasmPool() {
			continue
		}
		result = append(result, route)
	}

	if len(rankedRoutes) > 1 && len(result) == 0 {
		// If there are more than one routes and all of them are generalized cosmwasm pools,
		// then we return the top route.
		result = append(result, rankedRoutes[0])
	}

	return result
}

// createCandidateRouteByPoolID constructs a candidate route with the desired pool.
func (r *routerUseCaseImpl) createCandidateRouteByPoolID(tokenInDenom string, tokenOutDenom string, poolID uint64) ingesttypes.CandidateRoutes {
	// Create a candidate route with the desired pool
	return ingesttypes.CandidateRoutes{
		Routes: []ingesttypes.CandidateRoute{
			{
				Pools: []ingesttypes.CandidatePool{
					{
						ID:            poolID,
						TokenInDenom:  tokenInDenom,
						TokenOutDenom: tokenOutDenom,
					},
				},
			},
		},
		UniquePoolIDs: map[uint64]struct{}{
			poolID: {},
		},
	}
}
