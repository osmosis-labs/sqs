package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/osmoutils"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

var (
	_ mvc.RouterUsecase = &routerUseCaseImpl{}
)

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
	sortedPools   []sqsdomain.PoolI

	candidateRouteCache *cache.Cache
}

const (
	candidateRouteCacheLabel = "candidate_route"
	rankedRouteCacheLabel    = "ranked_route"

	// for candidate route cache, there is no order of magnitude
	noOrderOfMagnitude = ""

	// denomSeparatorChar = "|"
)

var (
	cacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_routes_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"route", "cache_type", "token_in", "token_out", "token_in_order_of_magnitude"},
	)
	cacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_routes_cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"route", "cache_type", "token_in", "token_out", "token_in_order_of_magnitude"},
	)
	cacheWrite = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_routes_cache_write_total",
			Help: "Total number of cache writes",
		},
		[]string{"route", "cache_type", "token_in", "token_out", "token_in_order_of_magnitude"},
	)

	zero = sdk.ZeroInt()
)

func init() {
	prometheus.MustRegister(cacheHits)
	prometheus.MustRegister(cacheMisses)
}

// NewRouterUsecase will create a new pools use case object
func NewRouterUsecase(tokensRepository mvc.RouterRepository, poolsUsecase mvc.PoolsUsecase, candidateRouteSearcher domain.CandidateRouteSearcher, tokenMetadataHolder mvc.TokenMetadataHolder, config domain.RouterConfig, cosmWasmPoolsConfig domain.CosmWasmPoolRouterConfig, logger log.Logger, rankedRouteCache *cache.Cache, candidateRouteCache *cache.Cache) mvc.RouterUsecase {
	return &routerUseCaseImpl{
		routerRepository:       tokensRepository,
		poolsUsecase:           poolsUsecase,
		tokenMetadataHolder:    tokenMetadataHolder,
		defaultConfig:          config,
		cosmWasmPoolsConfig:    cosmWasmPoolsConfig,
		candidateRouteSearcher: candidateRouteSearcher,
		logger:                 logger,

		rankedRouteCache:    rankedRouteCache,
		candidateRouteCache: candidateRouteCache,

		sortedPools:   make([]sqsdomain.PoolI, 0),
		sortedPoolsMu: sync.RWMutex{},
	}
}

// GetOptimalQuote returns the optimal quote by estimating the optimal route(s) through pools
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
func (r *routerUseCaseImpl) GetOptimalQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
	options := domain.RouterOptions{
		MaxPoolsPerRoute:                 r.defaultConfig.MaxPoolsPerRoute,
		MaxRoutes:                        r.defaultConfig.MaxRoutes,
		MinPoolLiquidityCap:              r.defaultConfig.MinPoolLiquidityCap,
		CandidateRouteCacheExpirySeconds: r.defaultConfig.CandidateRouteCacheExpirySeconds,
		RankedRouteCacheExpirySeconds:    r.defaultConfig.RankedRouteCacheExpirySeconds,
		MaxSplitRoutes:                   r.defaultConfig.MaxSplitRoutes,
	}
	// Apply options
	for _, opt := range opts {
		opt(&options)
	}

	// Get an order of magnitude for the token in amount
	// This is used for caching ranked routes as these might differ depending on the amount swapped in.
	tokenInOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenIn.Amount)

	candidateRankedRoutes, err := r.GetCachedRankedRoutes(ctx, tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude)
	if err != nil {
		return nil, err
	}

	var (
		topSingleRouteQuote domain.Quote
		rankedRoutes        []route.RouteImpl
	)

	// If no cached candidate routes are found, we attempt to
	// compute them.
	if len(candidateRankedRoutes.Routes) == 0 {
		// Get the dynamic min pool liquidity cap for the given token in and token out denoms.
		dynamicMinPoolLiquidityCap, err := r.tokenMetadataHolder.GetMinPoolLiquidityCap(tokenIn.Denom, tokenOutDenom)
		if err == nil {
			// Set the dynamic min pool liquidity cap only if there is no error retrieving it.
			// Otherwise, use the default.
			options.MinPoolLiquidityCap = r.ConvertMinTokensPoolLiquidityCapToFilter(dynamicMinPoolLiquidityCap)
		}

		// Find candidate routes and rank them by direct quotes.
		topSingleRouteQuote, rankedRoutes, err = r.computeAndRankRoutesByDirectQuote(ctx, tokenIn, tokenOutDenom, options)
		if err != nil {
			return nil, err
		}
	} else {
		// Otherwise, simply compute quotes over cached ranked routes
		topSingleRouteQuote, rankedRoutes, err = r.rankRoutesByDirectQuote(ctx, candidateRankedRoutes, tokenIn, tokenOutDenom, options.MaxSplitRoutes)
		if err != nil {
			return nil, err
		}
	}

	if len(rankedRoutes) == 1 || options.MaxSplitRoutes == domain.DisableSplitRoutes {
		return topSingleRouteQuote, nil
	}

	// Filter out generalized cosmWasm pool routes
	rankedRoutes = filterOutGeneralizedCosmWasmPoolRoutes(rankedRoutes)

	// If filtering leads to a single route left, return it.
	if len(rankedRoutes) == 1 {
		return topSingleRouteQuote, nil
	}

	// Compute split route quote
	topSplitQuote, err := getSplitQuote(ctx, rankedRoutes, tokenIn)
	if err != nil {
		// If error occurs in splits, return the single route quote
		// rather than failing.
		return topSingleRouteQuote, nil
	}

	finalQuote := topSingleRouteQuote

	// If the split route quote is better than the single route quote, return the split route quote
	if topSplitQuote.GetAmountOut().GT(topSingleRouteQuote.GetAmountOut()) {
		routes := topSplitQuote.GetRoute()

		r.logger.Debug("split route selected", zap.Int("route_count", len(routes)))

		finalQuote = topSplitQuote
	}

	r.logger.Debug("single route selected", zap.Stringer("route", finalQuote.GetRoute()[0]))

	if finalQuote.GetAmountOut().IsZero() {
		return nil, errors.New("best we can do is no tokens out")
	}

	return finalQuote, nil
}

// GetSimpleQuote implements mvc.RouterUsecase.
// TODO: cover with a simple test.
func (r *routerUseCaseImpl) GetSimpleQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
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
		options.MinPoolLiquidityCap = r.ConvertMinTokensPoolLiquidityCapToFilter(dynamicMinPoolLiquidityCap)
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
	}
	candidateRoutes, err := r.candidateRouteSearcher.FindCandidateRoutes(tokenIn, tokenOutDenom, candidateRouteSearchOptions)
	if err != nil {
		r.logger.Error("error getting candidate routes for pricing", zap.Error(err))
		return nil, err
	}

	routes, err := r.poolsUsecase.GetRoutesFromCandidates(candidateRoutes, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		r.logger.Error("error ranking routes for pricing", zap.Error(err))
		return nil, err
	}

	topQuote, _, err := r.estimateAndRankSingleRouteQuote(ctx, routes, tokenIn, r.logger)
	if err != nil {
		return nil, fmt.Errorf("%s, tokenOutDenom (%s)", err, tokenOutDenom)
	}

	return topQuote, nil
}

// filterAndConvertDuplicatePoolIDRankedRoutes filters ranked routes that contain duplicate pool IDs.
// Routes with overlapping Alloyed and transmuter pools are not filtered out.
// Additionally, the routes are converted into route.Route.Impl type.
// CONTRACT: rankedRoutes are sorted in decreasing order by amount out
// from first to last.
func filterAndConvertDuplicatePoolIDRankedRoutes(rankedRoutes []RouteWithOutAmount) []route.RouteImpl {
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

// rankRoutesByDirectQuote ranks the given candidate routes by estimating direct quotes over each route.
// Additionally, it fileters out routes with duplicate pool IDs and cuts them for splits
// based on the value of maxSplitRoutes.
// Returns the top quote as well as the ranked routes in decrease order of amount out.
// Returns error if:
// - fails to read taker fees
// - fails to convert candidate routes to routes
// - fails to estimate direct quotes
func (r *routerUseCaseImpl) rankRoutesByDirectQuote(ctx context.Context, candidateRoutes sqsdomain.CandidateRoutes, tokenIn sdk.Coin, tokenOutDenom string, maxSplitRoutes int) (domain.Quote, []route.RouteImpl, error) {
	// Note that retrieving pools and taker fees is done in separate transactions.
	// This is fine because taker fees don't change often.
	routes, err := r.poolsUsecase.GetRoutesFromCandidates(candidateRoutes, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, nil, err
	}

	topQuote, routesWithAmtOut, err := r.estimateAndRankSingleRouteQuote(ctx, routes, tokenIn, r.logger)
	if err != nil {
		return nil, nil, fmt.Errorf("%s, tokenOutDenom (%s)", err, tokenOutDenom)
	}

	// Update ranked routes with filtered ranked routes
	routes = filterAndConvertDuplicatePoolIDRankedRoutes(routesWithAmtOut)

	// Cut routes for splits
	routes = cutRoutesForSplits(maxSplitRoutes, routes)

	return topQuote, routes, nil
}

// computeAndRankRoutesByDirectQuote computes candidate routes and ranks them by token out after estimating direct quotes.
func (r *routerUseCaseImpl) computeAndRankRoutesByDirectQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, routingOptions domain.RouterOptions) (domain.Quote, []route.RouteImpl, error) {
	tokenInOrderOfMagnitude := GetPrecomputeOrderOfMagnitude(tokenIn.Amount)

	candidateRouteSearchOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           routingOptions.MaxRoutes,
		MaxPoolsPerRoute:    routingOptions.MaxPoolsPerRoute,
		MinPoolLiquidityCap: routingOptions.MinPoolLiquidityCap,
	}

	// If top routes are not present in cache, retrieve unranked candidate routes
	candidateRoutes, err := r.handleCandidateRoutes(ctx, tokenIn, tokenOutDenom, candidateRouteSearchOptions)
	if err != nil {
		r.logger.Error("error handling routes", zap.Error(err))
		return nil, nil, err
	}

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}

	if len(candidateRoutes.Routes) > 0 {
		cacheWrite.WithLabelValues(requestURLPath, candidateRouteCacheLabel, tokenIn.Denom, tokenOutDenom, noOrderOfMagnitude).Inc()

		r.candidateRouteCache.Set(formatCandidateRouteCacheKey(tokenIn.Denom, tokenOutDenom), candidateRoutes, time.Duration(routingOptions.CandidateRouteCacheExpirySeconds)*time.Second)
	} else {
		// If no candidate routes found, cache them for quarter of the duration
		r.candidateRouteCache.Set(formatCandidateRouteCacheKey(tokenIn.Denom, tokenOutDenom), candidateRoutes, time.Duration(routingOptions.CandidateRouteCacheExpirySeconds/4)*time.Second)

		r.rankedRouteCache.Set(formatRankedRouteCacheKey(tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude), candidateRoutes, time.Duration(routingOptions.RankedRouteCacheExpirySeconds/4)*time.Second)

		return nil, nil, fmt.Errorf("no candidate routes found")
	}

	// Rank candidate routes by estimating direct quotes
	topSingleRouteQuote, rankedRoutes, err := r.rankRoutesByDirectQuote(ctx, candidateRoutes, tokenIn, tokenOutDenom, routingOptions.MaxSplitRoutes)
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

		cacheWrite.WithLabelValues(requestURLPath, rankedRouteCacheLabel, tokenIn.Denom, tokenOutDenom, strconv.FormatInt(int64(tokenInOrderOfMagnitude), 10)).Inc()

		r.rankedRouteCache.Set(formatRankedRouteCacheKey(tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude), convertedCandidateRoutes, time.Duration(routingOptions.RankedRouteCacheExpirySeconds)*time.Second)
	}

	return topSingleRouteQuote, rankedRoutes, nil
}

var (
	ErrTokenInDenomPoolNotFound  = fmt.Errorf("token in denom not found in pool")
	ErrTokenOutDenomPoolNotFound = fmt.Errorf("token out denom not found in pool")
)

// GetCustomDirectQuote implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCustomDirectQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolID uint64) (domain.Quote, error) {
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

	// Retrieve taker fee for the pool
	takerFee, ok := r.routerRepository.GetTakerFee(tokenIn.Denom, tokenOutDenom)
	if !ok {
		return nil, fmt.Errorf("taker fee not found for pool %d, denom in (%s), denom out (%s)", poolID, tokenIn.Denom, tokenOutDenom)
	}

	// Create a taker fee map with the taker fee for the pool
	takerFeeMap := sqsdomain.TakerFeeMap{}
	takerFeeMap.SetTakerFee(tokenIn.Denom, tokenOutDenom, takerFee)

	// create candidate routes with given token out denom and pool ID.
	candidateRoutes := r.createCandidateRouteByPoolID(tokenOutDenom, poolID)

	// Convert candidate route into a route with all the pool data
	routes, err := r.poolsUsecase.GetRoutesFromCandidates(candidateRoutes, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, err
	}

	// Compute direct quote
	bestSingleRouteQuote, _, err := r.estimateAndRankSingleRouteQuote(ctx, routes, tokenIn, r.logger)
	if err != nil {
		return nil, err
	}

	return bestSingleRouteQuote, nil
}

var ErrValidationFailed = fmt.Errorf("validation failed")

// GetCustomDirectQuoteMultiPool implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCustomDirectQuoteMultiPool(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom []string, poolIDs []uint64) (domain.Quote, error) {
	if len(poolIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one pool ID should be specified", ErrValidationFailed)
	}

	if len(tokenOutDenom) == 0 {
		return nil, fmt.Errorf("%w: at least one token out denom should be specified", ErrValidationFailed)
	}

	// for each given pool we expect to have provided token out denom
	if len(poolIDs) != len(tokenOutDenom) {
		return nil, fmt.Errorf("%w: number of pool ID should match number of out denom", ErrValidationFailed)
	}

	// AmountIn is the first token of the asset pair.
	result := quoteImpl{AmountIn: tokenIn}
	for i, v := range poolIDs {
		tokenOutDenom := tokenOutDenom[i]

		quote, err := r.GetCustomDirectQuote(ctx, tokenIn, tokenOutDenom, v)
		if err != nil {
			return nil, err
		}

		// the amountOut value is the amount out of last the tokenOutDenom
		result.AmountOut = quote.GetAmountOut()

		// append each pool to the route
		result.Route = append(result.Route, quote.GetRoute()...)

		tokenIn = sdk.NewCoin(tokenOutDenom, quote.GetAmountOut())
	}
	return &result, nil
}

// GetCandidateRoutes implements domain.RouterUsecase.
func (r *routerUseCaseImpl) GetCandidateRoutes(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (sqsdomain.CandidateRoutes, error) {
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

	candidateRoutes, err := r.handleCandidateRoutes(ctx, tokenIn, tokenOutDenom, candidateRouteSearchOptions)
	if err != nil {
		return sqsdomain.CandidateRoutes{}, err
	}

	return candidateRoutes, nil
}

// GetTakerFee implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetTakerFee(poolID uint64) ([]sqsdomain.TakerFeeForPair, error) {
	pool, err := r.poolsUsecase.GetPool(poolID)
	if err != nil {
		return []sqsdomain.TakerFeeForPair{}, err
	}

	poolDenoms := pool.GetPoolDenoms()

	result := make([]sqsdomain.TakerFeeForPair, 0)

	for i := range poolDenoms {
		for j := i + 1; j < len(poolDenoms); j++ {
			denom0 := poolDenoms[i]
			denom1 := poolDenoms[j]

			takerFee, ok := r.routerRepository.GetTakerFee(denom0, denom1)
			if !ok {
				return []sqsdomain.TakerFeeForPair{}, fmt.Errorf("taker fee not found for pool %d, denom in (%s), denom out (%s)", poolID, denom0, denom1)
			}

			result = append(result, sqsdomain.TakerFeeForPair{
				Denom0:   denom0,
				Denom1:   denom1,
				TakerFee: takerFee,
			})
		}
	}

	return result, nil
}

// GetCachedCandidateRoutes implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCachedCandidateRoutes(ctx context.Context, tokenInDenom string, tokenOutDenom string) (sqsdomain.CandidateRoutes, bool, error) {
	if !r.defaultConfig.RouteCacheEnabled {
		return sqsdomain.CandidateRoutes{}, false, nil
	}

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return sqsdomain.CandidateRoutes{}, false, err
	}

	cachedCandidateRoutes, found := r.candidateRouteCache.Get(formatCandidateRouteCacheKey(tokenInDenom, tokenOutDenom))
	if !found {
		// Increase cache misses
		cacheMisses.WithLabelValues(requestURLPath, candidateRouteCacheLabel, tokenInDenom, tokenOutDenom, noOrderOfMagnitude).Inc()

		return sqsdomain.CandidateRoutes{
			Routes:        []sqsdomain.CandidateRoute{},
			UniquePoolIDs: map[uint64]struct{}{},
		}, false, nil
	}

	cacheHits.WithLabelValues(requestURLPath, candidateRouteCacheLabel, tokenInDenom, tokenOutDenom, noOrderOfMagnitude).Inc()

	candidateRoutes, ok := cachedCandidateRoutes.(sqsdomain.CandidateRoutes)
	if !ok {
		return sqsdomain.CandidateRoutes{}, false, fmt.Errorf("error casting candidate routes from cache")
	}

	return candidateRoutes, true, nil
}

// GetCachedRankedRoutes implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCachedRankedRoutes(ctx context.Context, tokenInDenom string, tokenOutDenom string, tokenInOrderOfMagnitude int) (sqsdomain.CandidateRoutes, error) {
	if !r.defaultConfig.RouteCacheEnabled {
		return sqsdomain.CandidateRoutes{}, nil
	}

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return sqsdomain.CandidateRoutes{}, err
	}

	cachedRankedRoutes, found := r.rankedRouteCache.Get(formatRankedRouteCacheKey(tokenInDenom, tokenOutDenom, tokenInOrderOfMagnitude))
	if !found {
		// Increase cache misses
		cacheMisses.WithLabelValues(requestURLPath, rankedRouteCacheLabel, tokenInDenom, tokenOutDenom, strconv.FormatInt(int64(tokenInOrderOfMagnitude), 10)).Inc()

		return sqsdomain.CandidateRoutes{}, nil
	}

	cacheHits.WithLabelValues(requestURLPath, rankedRouteCacheLabel, tokenInDenom, tokenOutDenom, strconv.FormatInt(int64(tokenInOrderOfMagnitude), 10)).Inc()

	rankedRoutes, ok := cachedRankedRoutes.(sqsdomain.CandidateRoutes)
	if !ok {
		return sqsdomain.CandidateRoutes{}, fmt.Errorf("error casting candidate routes from cache")
	}

	return rankedRoutes, nil
}

// handleCandidateRoutes attempts to retrieve candidate routes from the cache. If no routes are cached, it will
// compute, persist in cache and return them.
// Returns routes on success
// Errors if:
// - there is an error retrieving routes from cache
// - there are no routes cached and there is an error computing them
// - fails to persist the computed routes in cache
func (r *routerUseCaseImpl) handleCandidateRoutes(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, candidateRouteSearchOptions domain.CandidateRouteSearchOptions) (candidateRoutes sqsdomain.CandidateRoutes, err error) {
	r.logger.Debug("getting routes")

	// Check cache for routes if enabled
	var isFoundCached bool
	if r.defaultConfig.RouteCacheEnabled {
		candidateRoutes, isFoundCached, err = r.GetCachedCandidateRoutes(ctx, tokenIn.Denom, tokenOutDenom)
		if err != nil {
			return sqsdomain.CandidateRoutes{}, err
		}
	}

	r.logger.Debug("cached routes", zap.Int("num_routes", len(candidateRoutes.Routes)))

	// If no routes are cached, find them
	if !isFoundCached {
		r.logger.Debug("calculating routes")

		candidateRoutes, err = r.candidateRouteSearcher.FindCandidateRoutes(tokenIn, tokenOutDenom, candidateRouteSearchOptions)
		if err != nil {
			r.logger.Error("error getting candidate routes for pricing", zap.Error(err))
			return sqsdomain.CandidateRoutes{}, err
		}

		r.logger.Info("calculated routes", zap.Int("num_routes", len(candidateRoutes.Routes)))

		// Persist routes
		if r.defaultConfig.RouteCacheEnabled {
			cacheDurationSeconds := r.defaultConfig.CandidateRouteCacheExpirySeconds
			if len(candidateRoutes.Routes) == 0 {
				// If there are no routes, we want to cache the result for a shorter duration
				// Add 1 to ensure that it is never 0 as zero signifies never clearing.
				cacheDurationSeconds = cacheDurationSeconds/4 + 1
			}

			r.logger.Debug("persisting routes", zap.Int("num_routes", len(candidateRoutes.Routes)))
			r.candidateRouteCache.Set(formatCandidateRouteCacheKey(tokenIn.Denom, tokenOutDenom), candidateRoutes, time.Duration(cacheDurationSeconds)*time.Second)
		}
	}

	return candidateRoutes, nil
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

	if err := parsing.StorePools(pools, tickModelMap, "pools.json"); err != nil {
		return domain.RouterState{}, err
	}

	takerFeesMap := r.routerRepository.GetAllTakerFees()

	candidateRouteSearchData := r.routerRepository.GetCandidateRouteSearchData()

	return domain.RouterState{
		Pools:                    pools,
		TakerFees:                takerFeesMap,
		TickMap:                  tickModelMap,
		CandidateRouteSearchData: candidateRouteSearchData,
	}, nil
}

// formatRouteCacheKey formats the given token in and token out denoms to a string.
func formatRouteCacheKey(tokenInDenom string, tokenOutDenom string) string {
	return fmt.Sprintf("%s|%s", tokenInDenom, tokenOutDenom)
}

// formatRankedRouteCacheKey formats the given token in and token out denoms and order of magnitude to a string.
func formatRankedRouteCacheKey(tokenInDenom string, tokenOutDenom string, tokenIOrderOfMagnitude int) string {
	return fmt.Sprintf("%s|%d", formatRouteCacheKey(tokenInDenom, tokenOutDenom), tokenIOrderOfMagnitude)
}

// formatCandidateRouteCacheKey formats the given token in and token out denoms to a string.
func formatCandidateRouteCacheKey(tokenInDenom string, tokenOutDenom string) string {
	return fmt.Sprintf("cr%s", formatRouteCacheKey(tokenInDenom, tokenOutDenom))
}

// convertRankedToCandidateRoutes converts the given ranked routes to candidate routes.
// The primary use case for this is to keep minimal data for caching.
func convertRankedToCandidateRoutes(rankedRoutes []route.RouteImpl) sqsdomain.CandidateRoutes {
	candidateRoutes := sqsdomain.CandidateRoutes{
		Routes:                     make([]sqsdomain.CandidateRoute, 0, len(rankedRoutes)),
		UniquePoolIDs:              map[uint64]struct{}{},
		ContainsCanonicalOrderbook: false,
	}

	for _, rankedRoute := range rankedRoutes {
		candidateRoute := sqsdomain.CandidateRoute{
			Pools:                     make([]sqsdomain.CandidatePool, 0, len(rankedRoute.GetPools())),
			IsCanonicalOrderboolRoute: rankedRoute.HasCanonicalOrderbookPool,
		}

		candidateRoutes.ContainsCanonicalOrderbook = candidateRoutes.ContainsCanonicalOrderbook || rankedRoute.HasCanonicalOrderbookPool

		for _, randkedPool := range rankedRoute.GetPools() {
			candidatePool := sqsdomain.CandidatePool{
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

// SetSortedPools implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) SetSortedPools(pools []sqsdomain.PoolI) {
	r.sortedPoolsMu.Lock()
	r.sortedPools = pools
	r.sortedPoolsMu.Unlock()
}

// SetTakerFees implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) SetTakerFees(takerFees sqsdomain.TakerFeeMap) {
	r.routerRepository.SetTakerFees(takerFees)
}

// GetSortedPools implements mvc.RouterUsecase.
// Note that this method is not thread safe.
func (r *routerUseCaseImpl) GetSortedPools() []sqsdomain.PoolI {
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
func (r *routerUseCaseImpl) createCandidateRouteByPoolID(tokenOutDenom string, poolID uint64) sqsdomain.CandidateRoutes {
	// Create a candidate route with the desired pool
	return sqsdomain.CandidateRoutes{
		Routes: []sqsdomain.CandidateRoute{
			{
				Pools: []sqsdomain.CandidatePool{
					{
						ID:            poolID,
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
