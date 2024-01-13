package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/osmoutils"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v21/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"
	"github.com/osmosis-labs/sqs/sqsdomain"
	routerredisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis/router"
	"github.com/osmosis-labs/sqs/sqsutil"
)

var _ mvc.RouterUsecase = &routerUseCaseImpl{}

type routerUseCaseImpl struct {
	contextTimeout   time.Duration
	routerRepository routerredisrepo.RouterRepository
	poolsUsecase     mvc.PoolsUsecase
	config           domain.RouterConfig
	logger           log.Logger

	routesOverwrite *cache.RoutesOverwrite

	rankedRouteCache *cache.Cache

	// This is a path where the overwrite routes are stored as backup in case of failure.
	// On restart, the routes are loaded from this path.
	// It is defined on the use case for testability (s.t. we can set a temp path in tests)
	overwriteRoutesPath string
}

const (
	candidateRouteCacheLabel = "candidate_route"
	rankedRouteCacheLabel    = "ranked_route"

	denomSeparatorChar = "|"
)

var (
	cacheHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"route", "cache_type", "token_in", "token_out"},
	)
	cacheMisses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"route", "cache_type", "token_in", "token_out"},
	)
)

func init() {
	prometheus.MustRegister(cacheHits)
	prometheus.MustRegister(cacheMisses)
}

// NewRouterUsecase will create a new pools use case object
func NewRouterUsecase(timeout time.Duration, routerRepository routerredisrepo.RouterRepository, poolsUsecase mvc.PoolsUsecase, config domain.RouterConfig, logger log.Logger, rankedRouteCache *cache.Cache, routesOverwrite *cache.RoutesOverwrite) mvc.RouterUsecase {
	return &routerUseCaseImpl{
		contextTimeout:   timeout,
		routerRepository: routerRepository,
		poolsUsecase:     poolsUsecase,
		config:           config,
		logger:           logger,

		rankedRouteCache: rankedRouteCache,
		routesOverwrite:  routesOverwrite,
	}
}

// WithOverwriteRoutesPath sets the overwrite routes path on the router use case.
func WithOverwriteRoutesPath(routerUsecase mvc.RouterUsecase, overwriteRoutesPath string) mvc.RouterUsecase {
	useCaseImpl, ok := routerUsecase.(*routerUseCaseImpl)
	if !ok {
		panic("error casting router use case to router use case impl")
	}
	useCaseImpl.overwriteRoutesPath = overwriteRoutesPath
	return routerUsecase
}

// GetOptimalQuote returns the optimal quote by estimating the optimal route(s) through pools
// on the osmosis network.
// Uses caching strategies for optimal performance.
// Currently, supports candidate route caching. If candidate routes for the given token in and token out denoms
// are present in cache, they are used without re-computing them. Otherwise, they are computed and cached.
// In the future, we will support caching of ranked routes that are constructed from candidate and sorted
// by the decreasing amount out within an order of magnitude of token in. Similarly, We will also support optimal split caching
// Returns error if:
// - fails to estimate direct quotes for ranked routes
// - fails to retrieve candidate routes
func (r *routerUseCaseImpl) GetOptimalQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (domain.Quote, error) {
	preferredRouteCacheKey := formatRouteCacheKey(tokenIn.Denom, tokenOutDenom)
	preferredRoute, hasPreferredRoute := r.routesOverwrite.Get(preferredRouteCacheKey)

	// Get an order of magnitude for the token in amount
	// This is used for caching ranked routes as these might differ depending on the amount swapped in.
	tokenInOrderOfMagnitude := osmomath.OrderOfMagnitude(tokenIn.Amount.ToLegacyDec())

	// If no preferred route is found, check if we have ranked routes in cache
	if !hasPreferredRoute {
		preferredRoute, hasPreferredRoute = r.rankedRouteCache.Get(formatRankedRouteCacheKey(tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude))
	}

	var (
		rankedRoutes        []route.RouteImpl
		topSingleRouteQuote domain.Quote
		err                 error
	)

	router := r.initializeRouter()

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return nil, err
	}

	// Preferred route in this context is either an overwrite or a cached ranked route.
	// If an overwrite exists, it is always used over the ranked route.
	if hasPreferredRoute {
		// Increase cache hits
		cacheHits.WithLabelValues(requestURLPath, rankedRouteCacheLabel, tokenIn.Denom, tokenOutDenom).Inc()

		preferredRankedRoutes, ok := preferredRoute.(sqsdomain.CandidateRoutes)
		if !ok {
			return nil, fmt.Errorf("error casting ranked routes from cache")
		}

		// If top routes are present in cache, estimate the quotes and return the best.
		topSingleRouteQuote, rankedRoutes, err = r.rankRoutesByDirectQuote(ctx, router, preferredRankedRoutes, tokenIn, tokenOutDenom)
		if err != nil {
			return nil, err
		}
	} else {
		// Increase cache misses
		cacheMisses.WithLabelValues(requestURLPath, rankedRouteCacheLabel, tokenIn.Denom, tokenOutDenom).Inc()

		// If top routes are not present in cache, retrieve unranked candidate routes
		candidateRoutes, err := r.handleCandidateRoutes(ctx, router, tokenIn.Denom, tokenOutDenom)
		if err != nil {
			r.logger.Error("error handling routes", zap.Error(err))
			return nil, err
		}

		for _, route := range candidateRoutes.Routes {
			r.logger.Debug("filtered_candidate_route", zap.Any("route", route))
		}

		// Rank candidate routes by estimating direct quotes
		topSingleRouteQuote, rankedRoutes, err = r.rankRoutesByDirectQuote(ctx, router, candidateRoutes, tokenIn, tokenOutDenom)
		if err != nil {
			r.logger.Error("error getting top routes", zap.Error(err))
			return nil, err
		}

		if len(rankedRoutes) == 0 {
			return nil, fmt.Errorf("no ranked routes found")
		}

		// Update ranked routes with filtered ranked routes
		rankedRoutes = filterDuplicatePoolIDRoutes(rankedRoutes)

		if len(rankedRoutes) > 0 {
			// Convert ranked routes back to candidate for caching
			candidateRoutes = convertRankedToCandidateRoutes(rankedRoutes)

			// TODO move cache value to config.
			r.rankedRouteCache.Set(formatRankedRouteCacheKey(tokenIn.Denom, tokenOutDenom, tokenInOrderOfMagnitude), candidateRoutes, time.Minute*5)
		}
	}

	if len(rankedRoutes) == 1 {
		return topSingleRouteQuote, nil
	}

	// Compute split route quote
	topSplitQuote, err := router.GetSplitQuote(ctx, rankedRoutes, tokenIn)
	if err != nil {
		return nil, err
	}

	// TODO: Cache split route proportions

	finalQuote := topSingleRouteQuote

	// If the split route quote is better than the single route quote, return the split route quote
	if topSplitQuote.GetAmountOut().GT(topSingleRouteQuote.GetAmountOut()) {
		routes := topSplitQuote.GetRoute()

		r.logger.Debug("split route selected", zap.Int("route_count", len(routes)))
		for _, route := range routes {
			r.logger.Debug("route", zap.Stringer("route", route))
		}

		finalQuote = topSplitQuote
	}

	r.logger.Debug("single route selected", zap.Stringer("route", finalQuote.GetRoute()[0]))

	if finalQuote.GetAmountOut().IsZero() {
		return nil, errors.New("best we can do is no tokens out")
	}

	return finalQuote, nil
}

// filterDuplicatePoolIDRoutes filters routes that contain duplicate pool IDs.
// CONTRACT: rankedRoutes are sorted in decreasing order by amount out
// from first to last.
func filterDuplicatePoolIDRoutes(rankedRoutes []route.RouteImpl) []route.RouteImpl {
	// We use two maps for all routes and for the current route.
	// This is so that if a route ends up getting filtered, its pool IDs are not added to the combined map.
	combinedPoolIDsMap := make(map[uint64]struct{})
	filteredRankedRoutes := make([]route.RouteImpl, 0)

	for _, route := range rankedRoutes {
		pools := route.GetPools()

		currentRoutePoolIDsMap := make(map[uint64]struct{})

		existsPoolID := false

		for _, pool := range pools {
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
		filteredRankedRoutes = append(filteredRankedRoutes, route)
	}
	return filteredRankedRoutes
}

// rankRoutesByDirectQuote ranks the given candidate routes by estimating direct quotes over each route.
// Returns the top quote as well as the ranked routes in decrease order of amount out.
// Returns error if:
// - fails to read taker fees
// - fails to convert candidate routes to routes
// - fails to estimate direct quotes
func (r *routerUseCaseImpl) rankRoutesByDirectQuote(ctx context.Context, router *Router, candidateRoutes sqsdomain.CandidateRoutes, tokenIn sdk.Coin, tokenOutDenom string) (domain.Quote, []route.RouteImpl, error) {
	// Note that retrieving pools and taker fees is done in separate transactions.
	// This is fine because taker fees don't change often.
	// TODO: this can be refactored to only retrieve the relevant taker fees.
	takerFees, err := r.routerRepository.GetAllTakerFees(ctx)
	if err != nil {
		return nil, nil, err
	}

	routes, err := r.poolsUsecase.GetRoutesFromCandidates(ctx, candidateRoutes, takerFees, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, nil, err
	}

	topQuote, routes, err := estimateDirectQuote(ctx, router, routes, tokenIn)
	if err != nil {
		return nil, nil, err
	}

	return topQuote, routes, nil
}

// estimateDirectQuote estimates and returns the direct quote for the given routes, token in and token out denom.
// Also, returns the routes ranked by amount out in decreasing order.
// Returns error if:
// - fails to estimate direct quotes
func estimateDirectQuote(ctx context.Context, router *Router, routes []route.RouteImpl, tokenIn sdk.Coin) (domain.Quote, []route.RouteImpl, error) {
	topQuote, routesSortedByAmtOut, err := router.estimateAndRankSingleRouteQuote(ctx, routes, tokenIn)
	if err != nil {
		return nil, nil, err
	}

	numRoutes := len(routesSortedByAmtOut)

	// If split routes are disabled, return a single the top route
	if router.maxSplitRoutes == 0 && numRoutes > 0 {
		numRoutes = 1
		// If there are more routes than the max split routes, keep only the top routes
	} else if len(routesSortedByAmtOut) > router.maxSplitRoutes {
		// Keep only top routes for splits
		routes = routes[:router.maxSplitRoutes]
		numRoutes = router.maxSplitRoutes
	}

	// Convert routes sorted by amount out to routes
	for i := 0; i < numRoutes; i++ {
		// Update routes with the top routes
		routes[i] = routesSortedByAmtOut[i].RouteImpl
	}

	return topQuote, routes, nil
}

// GetBestSingleRouteQuote returns the best single route quote to be done directly without a split.
func (r *routerUseCaseImpl) GetBestSingleRouteQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (domain.Quote, error) {
	router := r.initializeRouter()

	candidateRoutes, err := r.handleCandidateRoutes(ctx, router, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, err
	}
	// TODO: abstract this

	takerFees, err := r.routerRepository.GetAllTakerFees(ctx)
	if err != nil {
		return nil, err
	}

	routes, err := r.poolsUsecase.GetRoutesFromCandidates(ctx, candidateRoutes, takerFees, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, err
	}

	return router.getBestSingleRouteQuote(ctx, tokenIn, routes)
}

// GetCustomQuote implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCustomQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolIDs []uint64) (domain.Quote, error) {
	// TODO: abstract this
	router := r.initializeRouter()

	candidateRoutes, err := r.handleCandidateRoutes(ctx, router, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, err
	}

	takerFees, err := r.routerRepository.GetAllTakerFees(ctx)
	if err != nil {
		return nil, err
	}

	routes, err := r.poolsUsecase.GetRoutesFromCandidates(ctx, candidateRoutes, takerFees, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, err
	}

	routeIndex := -1

	for curRouteIndex, route := range routes {
		routePools := route.GetPools()

		// Skip routes that do not match the pool length.
		if len(routePools) != len(poolIDs) {
			continue
		}

		for i, pool := range routePools {
			poolID := pool.GetId()

			desiredPoolID := poolIDs[i]

			// Break out of the loop if the poolID does not match the desired poolID
			if poolID != desiredPoolID {
				break
			}

			// Found a route that matches the poolIDs
			if i == len(routePools)-1 {
				routeIndex = curRouteIndex
			}
		}

		// If the routeIndex is not -1, then we found a route that matches the poolIDs
		// Break out of the loop
		if routeIndex != -1 {
			break
		}
	}

	// Validate routeIndex
	if routeIndex == -1 {
		return nil, fmt.Errorf("no route found for poolIDs: %v", poolIDs)
	}
	if routeIndex >= len(routes) {
		return nil, fmt.Errorf("routeIndex %d is out of bounds", routeIndex)
	}

	// Compute direct quote
	foundRoute := routes[routeIndex]
	quote, _, err := router.estimateAndRankSingleRouteQuote(ctx, []route.RouteImpl{foundRoute}, tokenIn)
	if err != nil {
		return nil, err
	}

	return quote, nil
}

// GetCustomDirectQuote implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetCustomDirectQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolID uint64) (domain.Quote, error) {
	pool, err := r.poolsUsecase.GetPool(ctx, poolID)
	if err != nil {
		return nil, err
	}

	poolDenoms := pool.GetPoolDenoms()

	if !osmoutils.Contains(poolDenoms, tokenIn.Denom) {
		return nil, fmt.Errorf("token in denom %s not found in pool %d", tokenIn.Denom, poolID)
	}
	if !osmoutils.Contains(poolDenoms, tokenOutDenom) {
		return nil, fmt.Errorf("token out denom %s not found in pool %d", tokenOutDenom, poolID)
	}

	// Retrieve taker fee for the pool
	takerFee, err := r.routerRepository.GetTakerFee(ctx, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, err
	}

	// Create a taker fee map with the taker fee for the pool
	takerFeeMap := sqsdomain.TakerFeeMap{}
	takerFeeMap.SetTakerFee(tokenIn.Denom, tokenOutDenom, takerFee)

	// Create a candidate route with the desired pool
	candidateRoutes := sqsdomain.CandidateRoutes{
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

	// Convert candidate route into a route with all the pool data
	routes, err := r.poolsUsecase.GetRoutesFromCandidates(ctx, candidateRoutes, takerFeeMap, tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return nil, err
	}

	// Compute direct quote
	router := r.initializeRouter()
	return router.getBestSingleRouteQuote(ctx, tokenIn, routes)
}

// GetCandidateRoutes implements domain.RouterUsecase.
func (r *routerUseCaseImpl) GetCandidateRoutes(ctx context.Context, tokenInDenom string, tokenOutDenom string) (sqsdomain.CandidateRoutes, error) {
	router := r.initializeRouter()

	routes, err := r.handleCandidateRoutes(ctx, router, tokenInDenom, tokenOutDenom)
	if err != nil {
		return sqsdomain.CandidateRoutes{}, err
	}

	return routes, nil
}

// GetTakerFee implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetTakerFee(ctx context.Context, poolID uint64) ([]sqsdomain.TakerFeeForPair, error) {
	takerFees, err := r.routerRepository.GetAllTakerFees(ctx)
	if err != nil {
		return []sqsdomain.TakerFeeForPair{}, err
	}

	pool, err := r.poolsUsecase.GetPool(ctx, poolID)
	if err != nil {
		return []sqsdomain.TakerFeeForPair{}, err
	}

	poolDenoms := pool.GetPoolDenoms()

	result := make([]sqsdomain.TakerFeeForPair, 0)

	for i := range poolDenoms {
		for j := i + 1; j < len(poolDenoms); j++ {
			denom0 := poolDenoms[i]
			denom1 := poolDenoms[j]

			takerFee := takerFees.GetTakerFee(denom0, denom1)

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
func (r *routerUseCaseImpl) GetCachedCandidateRoutes(ctx context.Context, tokenInDenom string, tokenOutDenom string) (sqsdomain.CandidateRoutes, error) {
	if !r.config.RouteCacheEnabled {
		return sqsdomain.CandidateRoutes{}, fmt.Errorf("route cache is disabled")
	}

	cachedCandidateRoutes, err := r.routerRepository.GetRoutes(ctx, tokenInDenom, tokenOutDenom)
	if err != nil {
		return sqsdomain.CandidateRoutes{}, err
	}

	return cachedCandidateRoutes, nil
}

// GetConfig implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetConfig() domain.RouterConfig {
	return r.config
}

// initializeRouter initializes the router per configuration defined on the use case
// Returns error if:
// - there is an error retrieving pools from the store
// - there is an error retrieving taker fees from the store
// TODO: test
func (r *routerUseCaseImpl) initializeRouter() *Router {
	router := NewRouter([]uint64{}, r.config.MaxPoolsPerRoute, r.config.MaxRoutes, r.config.MaxSplitRoutes, r.config.MaxSplitIterations, r.config.MinOSMOLiquidity, r.logger)
	router = WithRouterRepository(router, r.routerRepository)
	router = WithPoolsUsecase(router, r.poolsUsecase)

	return router
}

// handleCandidateRoutes attempts to retrieve candidate routes from the cache. If no routes are cached, it will
// compute, persist in cache and return them.
// Returns routes on success
// Errors if:
// - there is an error retrieving routes from cache
// - there are no routes cached and there is an error computing them
// - fails to persist the computed routes in cache
func (r *routerUseCaseImpl) handleCandidateRoutes(ctx context.Context, router *Router, tokenInDenom, tokenOutDenom string) (candidateRoutes sqsdomain.CandidateRoutes, err error) {
	r.logger.Debug("getting routes")

	// Check cache for routes if enabled
	if r.config.RouteCacheEnabled {
		candidateRoutes, err = r.routerRepository.GetRoutes(ctx, tokenInDenom, tokenOutDenom)
		if err != nil {
			return sqsdomain.CandidateRoutes{}, err
		}
	}

	r.logger.Debug("cached routes", zap.Int("num_routes", len(candidateRoutes.Routes)))

	// Get request path for metrics
	requestURLPath, err := domain.GetURLPathFromContext(ctx)
	if err != nil {
		return sqsdomain.CandidateRoutes{}, err
	}

	// If no routes are cached, find them
	if len(candidateRoutes.Routes) == 0 {
		// Increase cache misses
		cacheMisses.WithLabelValues(requestURLPath, candidateRouteCacheLabel, tokenInDenom, tokenOutDenom).Inc()

		r.logger.Debug("calculating routes")
		allPools, err := r.poolsUsecase.GetAllPools(ctx)
		if err != nil {
			return sqsdomain.CandidateRoutes{}, err
		}
		r.logger.Debug("retrieved pools", zap.Int("num_pools", len(allPools)))
		router = WithSortedPools(router, allPools)

		candidateRoutes, err = router.GetCandidateRoutes(tokenInDenom, tokenOutDenom)
		if err != nil {
			return sqsdomain.CandidateRoutes{}, err
		}

		r.logger.Info("calculated routes", zap.Int("num_routes", len(candidateRoutes.Routes)))

		// Persist routes
		if len(candidateRoutes.Routes) > 0 && r.config.RouteCacheEnabled {
			r.logger.Debug("persisting routes", zap.Int("num_routes", len(candidateRoutes.Routes)))
			if err := r.routerRepository.SetRoutes(ctx, tokenInDenom, tokenOutDenom, candidateRoutes); err != nil {
				return sqsdomain.CandidateRoutes{}, err
			}
		}
	} else {
		cacheHits.WithLabelValues(requestURLPath, candidateRouteCacheLabel, tokenInDenom, tokenOutDenom).Inc()
	}

	return candidateRoutes, nil
}

// StoreRouterStateFiles implements domain.RouterUsecase.
// TODO: clean up
func (r *routerUseCaseImpl) StoreRouterStateFiles(ctx context.Context) error {
	// These pools do not contain tick model
	pools, err := r.poolsUsecase.GetAllPools(ctx)

	if err != nil {
		return err
	}

	concentratedpoolIDs := make([]uint64, 0, len(pools))
	for _, pool := range pools {
		if pool.GetType() == poolmanagertypes.Concentrated {
			concentratedpoolIDs = append(concentratedpoolIDs, pool.GetId())
		}
	}

	tickModelMap, err := r.poolsUsecase.GetTickModelMap(ctx, concentratedpoolIDs)
	if err != nil {
		return err
	}

	if err := parsing.StorePools(pools, tickModelMap, "pools.json"); err != nil {
		return err
	}

	takerFeesMap, err := r.routerRepository.GetAllTakerFees(ctx)
	if err != nil {
		return err
	}

	if err := parsing.StoreTakerFees("taker_fees.json", takerFeesMap); err != nil {
		return err
	}

	return nil
}

// OverwriteRoutes implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) OverwriteRoutes(ctx context.Context, tokeinInDenom string, routes []sqsdomain.CandidateRoute) error {
	if len(routes) == 0 {
		return errors.New("routes cannot be empty")
	}

	// Find the unique pool IDs
	uniquePoolIDs := make(map[uint64]struct{})

	var (
		// The token out denom that we expect to be the same for all routes
		// We initialize it to token out denom of the first route and then validate
		// that it equals for all other routes.
		expectedTokenOutDenom string
		// The token out denom of the previous pool
		// For the first pool in route, assumed to be tokenInDenom
		previousPoolsTokenOutDenom = tokeinInDenom
	)
	for i, route := range routes {
		for _, pool := range route.Pools {
			// Validate that token in is present in the first pool
			poolData, err := r.poolsUsecase.GetPool(ctx, pool.ID)
			if err != nil {
				return err
			}

			poolDenoms := poolData.GetPoolDenoms()
			if !osmoutils.Contains(poolDenoms, previousPoolsTokenOutDenom) {
				return fmt.Errorf("token in denom %s not found in pool %d of route with index %d", tokeinInDenom, pool.ID, i)
			}

			// Persist unique pool ID
			uniquePoolIDs[pool.ID] = struct{}{}

			previousPoolsTokenOutDenom = pool.TokenOutDenom
		}

		// Make sure that the token out denom of the previous route is the same as for current route
		// That is, all routes have the same token out denom
		if i != 0 {
			if expectedTokenOutDenom != previousPoolsTokenOutDenom {
				return fmt.Errorf("token out denom %s does not match expected token out denom %s for route with index %d", previousPoolsTokenOutDenom, expectedTokenOutDenom, i)
			}
		}
		expectedTokenOutDenom = previousPoolsTokenOutDenom
	}

	// Create the overwrite data structure that we save in cache
	candidateRoutes := sqsdomain.CandidateRoutes{
		Routes:        routes,
		UniquePoolIDs: uniquePoolIDs,
	}

	// Note that we only persist overwrite in one direction (tokenInDenom -> tokenOutDenom)
	// For other directions, we must resubmit the request with denoms inversed.
	overwriteKey := formatRouteCacheKey(tokeinInDenom, expectedTokenOutDenom)
	r.routesOverwrite.Set(overwriteKey, candidateRoutes)

	// Also save this thing to a file for crash recovery
	bz, err := json.Marshal(candidateRoutes)
	if err != nil {
		return err
	}

	err = sqsutil.WriteBytes(r.overwriteRoutesPath, url.PathEscape(overwriteKey), bz)
	if err != nil {
		return err
	}

	return nil
}

// LoadOverwriteRoutes loads the overwrite routes from disk if they exist.
// If they do not exist, this is a no-op.
// If they exist, it loads them into the router usecase.
// Returns errors if any.
func (r *routerUseCaseImpl) LoadOverwriteRoutes(ctx context.Context) error {
	// Read overwrite routes from disk if they exist.
	_, err := os.Stat(r.overwriteRoutesPath)
	if err != nil {
		if os.IsNotExist(err) {
			// We do not have to create the path because we expect the first request
			// to create the directory.
			return nil
		}
	} else if err == nil {
		entries, err := os.ReadDir(r.overwriteRoutesPath)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				return fmt.Errorf("overwrite routes directory should not contain subdirectories")
			}

			fileName := entry.Name()

			// Read the entire file
			content, err := os.ReadFile(fmt.Sprintf("%s/%s", r.overwriteRoutesPath, fileName))
			if err != nil {
				return err
			}

			// Parse to candidate routes
			var candidateRoutes sqsdomain.CandidateRoutes
			if err := json.Unmarshal(content, &candidateRoutes); err != nil {
				return err
			}

			tokenInDenomTokenOutDenomStr, err := url.PathUnescape(fileName)
			if err != nil {
				return err
			}

			tokenInDenomTokenOutDenom := strings.Split(tokenInDenomTokenOutDenomStr, denomSeparatorChar)
			if len(tokenInDenomTokenOutDenom) != 2 {
				return fmt.Errorf("overwrite routes file name should be of format: '<tokenInDenom>%s<tokenOutDenom>.json URL-escaped", denomSeparatorChar)
			}

			tokenInDenom := tokenInDenomTokenOutDenom[0]

			if err := r.OverwriteRoutes(ctx, tokenInDenom, candidateRoutes.Routes); err != nil {
				return err
			}
		}
	}
	return nil
}

// formatRouteCacheKey formats the given token in and token out denoms to a string.
func formatRouteCacheKey(tokenInDenom string, tokenOutDenom string) string {
	return fmt.Sprintf("%s%s%s", tokenInDenom, denomSeparatorChar, tokenOutDenom)
}

// formatRankedRouteCacheKey formats the given token in and token out denoms and order of magnitude to a string.
func formatRankedRouteCacheKey(tokenInDenom string, tokenOutDenom string, tokenIOrderOfMagnitude int) string {
	return fmt.Sprintf("%s%s%d", formatRouteCacheKey(tokenInDenom, tokenOutDenom), denomSeparatorChar, tokenIOrderOfMagnitude)
}

// convertRankedToCandidateRoutes converts the given ranked routes to candidate routes.
// The primary use case for this is to keep minimal data for caching.
func convertRankedToCandidateRoutes(rankedRoutes []route.RouteImpl) sqsdomain.CandidateRoutes {
	candidateRoutes := sqsdomain.CandidateRoutes{
		Routes:        make([]sqsdomain.CandidateRoute, 0, len(rankedRoutes)),
		UniquePoolIDs: map[uint64]struct{}{},
	}

	for _, rankedRoute := range rankedRoutes {
		candidateRoute := sqsdomain.CandidateRoute{
			Pools: make([]sqsdomain.CandidatePool, 0, len(rankedRoute.GetPools())),
		}

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

// GetPoolSpotPrice implements mvc.RouterUsecase.
func (r *routerUseCaseImpl) GetPoolSpotPrice(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error) {
	poolTakerFee, err := r.routerRepository.GetTakerFee(ctx, quoteAsset, baseAsset)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	spotPrice, err := r.poolsUsecase.GetPoolSpotPrice(ctx, poolID, poolTakerFee, baseAsset, quoteAsset)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return spotPrice, nil
}
