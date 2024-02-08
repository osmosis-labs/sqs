package usecase_test

import (
	"context"
	"os"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/sqsdomain"
	sqsdomainmocks "github.com/osmosis-labs/sqs/sqsdomain/mocks"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v23/x/gamm/pool-models/balancer"
)

const (
	// See description of:
	// - TestGetOptimalQuote_Cache_Overwrites
	// - TestOverwriteRoutes
	// for details.
	poolIDOneBalancer      = uint64(1)
	poolID1135Concentrated = uint64(1135)
	poolID1265Concentrated = uint64(1265)
)

var (
	// For the purposes of testing cache, we focus on a small amount of token in (1_000_000 uosmo), expecting pool 1265 to be returned.
	// Search for tests that reference this value and read test description for details.
	defaultAmountInCache = osmomath.NewInt(1_000_000)

	// See description of:
	// - TestGetOptimalQuote_Cache_Overwrites
	// - TestOverwriteRoutes
	// for details.
	poolIDOneRoute = sqsdomain.CandidateRoutes{
		Routes: []sqsdomain.CandidateRoute{
			{
				Pools: []sqsdomain.CandidatePool{
					{
						ID:            poolIDOneBalancer,
						TokenOutDenom: ATOM,
					},
				},
			},
		},
		UniquePoolIDs: map[uint64]struct{}{
			poolIDOneBalancer: {},
		},
	}

	poolID1135Route = sqsdomain.CandidateRoutes{
		Routes: []sqsdomain.CandidateRoute{
			{
				Pools: []sqsdomain.CandidatePool{
					{
						ID:            poolID1135Concentrated,
						TokenOutDenom: ATOM,
					},
				},
			},
		},
		UniquePoolIDs: map[uint64]struct{}{
			poolID1135Concentrated: {},
		},
	}
)

// Tests the call to handleRoutes by mocking the router repository and pools use case
// with relevant data.
func (s *RouterTestSuite) TestHandleRoutes() {
	const (
		defaultTimeoutDuration = time.Second * 10

		tokenInDenom  = "uosmo"
		tokenOutDenom = "uion"

		minOsmoLiquidity = 10000 * usecase.OsmoPrecisionMultiplier
	)

	// Create test balancer pool

	balancerCoins := sdk.NewCoins(
		sdk.NewCoin(tokenInDenom, sdk.NewInt(1000000000000000000)),
		sdk.NewCoin(tokenOutDenom, sdk.NewInt(1000000000000000000)),
	)

	balancerPoolID := s.PrepareBalancerPoolWithCoins(balancerCoins...)
	balancerPool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, balancerPoolID)
	s.Require().NoError(err)

	defaultPool := &sqsdomain.PoolWrapper{
		ChainModel: balancerPool,
		SQSModel: sqsdomain.SQSPool{
			TotalValueLockedUSDC: osmomath.NewInt(int64(minOsmoLiquidity + 1)),
			PoolDenoms:           []string{tokenInDenom, tokenOutDenom},
			Balances:             balancerCoins,
			SpreadFactor:         DefaultSpreadFactor,
		},
	}

	var (
		defaultRoute = WithCandidateRoutePools(
			EmptyCandidateRoute,
			[]sqsdomain.CandidatePool{
				{
					ID:            defaultPool.GetId(),
					TokenOutDenom: tokenOutDenom,
				},
			},
		)

		defaultSinglePools = []sqsdomain.PoolI{defaultPool}

		singleDefaultRoutes = sqsdomain.CandidateRoutes{
			Routes: []sqsdomain.CandidateRoute{defaultRoute},
			UniquePoolIDs: map[uint64]struct{}{
				defaultPool.GetId(): {},
			},
		}

		emptyPools = []sqsdomain.PoolI{}

		emptyRoutes = sqsdomain.CandidateRoutes{}

		defaultRouterConfig = domain.RouterConfig{
			// Only these config values are relevant for this test
			// for searching for routes when none were present in cache.
			MaxPoolsPerRoute: 4,
			MaxRoutes:        4,

			// These configs are not relevant for this test.
			PreferredPoolIDs:          []uint64{},
			MaxSplitIterations:        10,
			MinOSMOLiquidity:          minOsmoLiquidity,
			RouteUpdateHeightInterval: 10,
		}
	)

	testCases := []struct {
		name string

		repositoryRoutes sqsdomain.CandidateRoutes
		repositoryPools  []sqsdomain.PoolI
		takerFeeMap      sqsdomain.TakerFeeMap
		isCacheDisabled  bool

		expectedCandidateRoutes sqsdomain.CandidateRoutes

		expectedError error
	}{
		{
			name: "routes in cache -> use them",

			repositoryRoutes: singleDefaultRoutes,
			repositoryPools:  emptyPools,

			expectedCandidateRoutes: singleDefaultRoutes,
		},
		{
			name: "cache is disabled in config -> recomputes routes despite having available in cache",

			repositoryRoutes: singleDefaultRoutes,
			repositoryPools:  emptyPools,
			isCacheDisabled:  true,

			expectedCandidateRoutes: emptyRoutes,
		},
		{
			name: "no routes in cache but relevant pools in store -> recomputes routes",

			repositoryRoutes: emptyRoutes,
			repositoryPools:  defaultSinglePools,

			expectedCandidateRoutes: singleDefaultRoutes,
		},
		{
			name: "no routes in cache and no relevant pools in store -> returns no routes",

			repositoryRoutes: emptyRoutes,
			repositoryPools:  emptyPools,

			expectedCandidateRoutes: emptyRoutes,
		},

		// TODO:
		// routes in cache but pools have more optimal -> cache is still used
		// multiple routes in cache -> use them
		// multiple rotues in pools -> use them
		// error in repository -> return error
		// error in storing routes after recomputing -> return error
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {

			routerRepositoryMock := &sqsdomainmocks.RedisRouterRepositoryMock{
				Routes: map[sqsdomain.DenomPair]sqsdomain.CandidateRoutes{
					// These are the routes that are stored in cache and returned by the call to GetRoutes.
					{Denom0: tokenOutDenom, Denom1: tokenInDenom}: tc.repositoryRoutes,
				},

				// No need to set taker fees on the mock since they are only relevant when
				// set on the router for this test.
			}

			poolsUseCaseMock := &mocks.PoolsUsecaseMock{
				// These are the pools returned by the call to GetAllPools
				Pools: tc.repositoryPools,
			}

			routerUseCase := usecase.NewRouterUsecase(defaultTimeoutDuration, routerRepositoryMock, poolsUseCaseMock, domain.RouterConfig{
				RouteCacheEnabled: !tc.isCacheDisabled,
			}, &log.NoOpLogger{}, cache.New(), cache.NewNoOpRoutesOverwrite())

			routerUseCaseImpl, ok := routerUseCase.(*usecase.RouterUseCaseImpl)
			s.Require().True(ok)

			// Initialize router
			router := usecase.NewRouter(defaultRouterConfig.PreferredPoolIDs, defaultRouterConfig.MaxPoolsPerRoute, defaultRouterConfig.MaxRoutes, defaultRouterConfig.MaxSplitRoutes, defaultRouterConfig.MaxSplitIterations, defaultRouterConfig.MaxSplitIterations, &log.NoOpLogger{})
			router = usecase.WithSortedPools(router, poolsUseCaseMock.Pools)

			// System under test
			ctx := context.Background()
			actualCandidateRoutes, err := routerUseCaseImpl.HandleRoutes(ctx, router, tokenInDenom, tokenOutDenom)

			if tc.expectedError != nil {
				s.Require().EqualError(err, tc.expectedError.Error())
				s.Require().Len(actualCandidateRoutes, 0)
				return
			}

			s.Require().NoError(err)

			// Pre-set routes should be returned.

			s.Require().Equal(len(tc.expectedCandidateRoutes.Routes), len(actualCandidateRoutes.Routes))
			for i, route := range actualCandidateRoutes.Routes {
				s.Require().Equal(tc.expectedCandidateRoutes.Routes[i], route)
			}

			// For the case where the cache is disabled, the expected routes in cache
			// will be the same as the original routes in the repository.
			if tc.isCacheDisabled {
				tc.expectedCandidateRoutes = tc.repositoryRoutes
			}

			// Check that router repository was updated
			s.Require().Equal(tc.expectedCandidateRoutes, routerRepositoryMock.Routes[sqsdomain.DenomPair{Denom0: tokenOutDenom, Denom1: tokenInDenom}])
		})
	}
}

// Tests that routes that overlap in pools IDs get filtered out.
// Tests that the order of the routes is in decreasing priority.
// That is, if routes A and B overlap where A comes before B, then B is filtered out.
// Additionally, tests that overlapping within the same route has no effect on fitlering.
// Lastly, validates that if a route overlaps with subsequent routes in the list but gets filtered out,
// then subesequent routes are not affected by filtering.
func (s *RouterTestSuite) TestFilterDuplicatePoolIDRoutes() {
	var (
		deafaultPool = &mocks.MockRoutablePool{ID: defaultPoolID}

		otherPool = &mocks.MockRoutablePool{ID: defaultPoolID + 1}

		defaultSingleRoute = WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
			deafaultPool,
		})
	)

	tests := map[string]struct {
		routes []route.RouteImpl

		expectedRoutes []route.RouteImpl
	}{
		"empty routes": {
			routes:         []route.RouteImpl{},
			expectedRoutes: []route.RouteImpl{},
		},

		"single route single pool": {
			routes: []route.RouteImpl{
				defaultSingleRoute,
			},

			expectedRoutes: []route.RouteImpl{
				defaultSingleRoute,
			},
		},

		"single route two different pools": {
			routes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					deafaultPool,
					otherPool,
				}),
			},

			expectedRoutes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					deafaultPool,
					otherPool,
				}),
			},
		},

		// Note that filtering only happens if pool ID duplciated across different routes.
		// Duplicate pool IDs within the same route are filtered out at a different step
		// in the router logic.
		"single route two same pools (have no effect on filtering)": {
			routes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					deafaultPool,
					deafaultPool,
				}),
			},

			expectedRoutes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					deafaultPool,
					deafaultPool,
				}),
			},
		},

		"two single hop routes and no duplicates": {
			routes: []route.RouteImpl{
				defaultSingleRoute,

				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					otherPool,
				}),
			},

			expectedRoutes: []route.RouteImpl{
				defaultSingleRoute,

				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					otherPool,
				}),
			},
		},

		"two single hop routes with duplicates (second filtered)": {
			routes: []route.RouteImpl{
				defaultSingleRoute,

				defaultSingleRoute,
			},

			expectedRoutes: []route.RouteImpl{
				defaultSingleRoute,
			},
		},

		"three route. first and second overlap. second and third overlap. second is filtered out but not third": {
			routes: []route.RouteImpl{
				defaultSingleRoute,

				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					deafaultPool, // first and second overlap
					otherPool,    // second and third overlap
				}),

				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					otherPool,
				}),
			},

			expectedRoutes: []route.RouteImpl{
				defaultSingleRoute,

				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					otherPool,
				}),
			},
		},
	}

	for name, tc := range tests {
		tc := tc
		s.Run(name, func() {

			actualRoutes := usecase.FilterDuplicatePoolIDRoutes(tc.routes)

			s.Require().Equal(len(tc.expectedRoutes), len(actualRoutes))
		})
	}
}

func (s *RouterTestSuite) TestConvertRankedToCandidateRoutes() {

	tests := map[string]struct {
		rankedRoutes []route.RouteImpl

		expectedCandidateRoutes sqsdomain.CandidateRoutes
	}{
		"empty ranked routes": {
			rankedRoutes: []route.RouteImpl{},

			expectedCandidateRoutes: sqsdomain.CandidateRoutes{
				Routes:        []sqsdomain.CandidateRoute{},
				UniquePoolIDs: map[uint64]struct{}{},
			},
		},
		"single route": {
			rankedRoutes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithPoolID(mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), &balancer.Pool{}), defaultPoolID),
				}),
			},

			expectedCandidateRoutes: sqsdomain.CandidateRoutes{
				Routes: []sqsdomain.CandidateRoute{
					WithCandidateRoutePools(sqsdomain.CandidateRoute{}, []sqsdomain.CandidatePool{
						{
							ID:            defaultPoolID,
							TokenOutDenom: DenomOne,
						},
					}),
				},
				UniquePoolIDs: map[uint64]struct{}{
					defaultPoolID: {},
				},
			},
		},
		"two routes": {
			rankedRoutes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithPoolID(mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), &balancer.Pool{}), defaultPoolID),
				}),
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithPoolID(mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), &balancer.Pool{}), defaultPoolID+1),
				}),
			},

			expectedCandidateRoutes: sqsdomain.CandidateRoutes{
				Routes: []sqsdomain.CandidateRoute{
					WithCandidateRoutePools(sqsdomain.CandidateRoute{}, []sqsdomain.CandidatePool{
						{
							ID:            defaultPoolID,
							TokenOutDenom: DenomOne,
						},
					}),
					WithCandidateRoutePools(sqsdomain.CandidateRoute{}, []sqsdomain.CandidatePool{
						{
							ID:            defaultPoolID + 1,
							TokenOutDenom: DenomOne,
						},
					}),
				},
				UniquePoolIDs: map[uint64]struct{}{
					defaultPoolID:     {},
					defaultPoolID + 1: {},
				},
			},
		},
	}

	for name, tc := range tests {
		tc := tc
		s.Run(name, func() {

			actualCandidateRoutes := usecase.ConvertRankedToCandidateRoutes(tc.rankedRoutes)

			s.Require().Equal(tc.expectedCandidateRoutes, actualCandidateRoutes)
		})
	}
}

// Validates that the ranked route cache functions as expected for optimal quotes.
// This test is set up by focusing on ATOM / OSMO mainnet state pool.
// We restrict the number of routes via config.
//
// As of today there are 3 major ATOM / OSMO pools:
// Pool ID 1: https://app.osmosis.zone/pool/1 (balancer) 0.2% spread factor and 20M of liquidity to date
// Pool ID 1135: https://app.osmosis.zone/pool/1135 (concentrated) 0.2% spread factor and 14M of liquidity to date
// Pool ID 1265: https://app.osmosis.zone/pool/1265 (concentrated) 0.05% spread factor and 224K of liquidity to date
//
// Based on this state, the small amounts of token in should go through pool 1265
// Medium amounts of token in should go through pool 1135
// and large amounts of token in should go through pool 1.
//
// For the purposes of testing cache, we focus on a small amount of token in (1_000_000 uosmo), expecting pool 1265 to be returned.
// We will, however, tweak the cache by test case to force other pools to be returned and ensure that the cache is used.
func (s *RouterTestSuite) TestGetOptimalQuote_Cache_Overwrites() {
	const (
		defaultTokenInDenom  = UOSMO
		defaultTokenOutDenom = ATOM
	)

	tests := map[string]struct {
		preCachedRoutes              sqsdomain.CandidateRoutes
		overwriteRoutes              sqsdomain.CandidateRoutes
		cacheOrderOfMagnitudeTokenIn int

		cacheExpiryDuration time.Duration

		amountIn osmomath.Int

		expectedRoutePoolID uint64
	}{
		"cache is not set, computes routes": {
			amountIn: defaultAmountInCache,

			// For the default amount in, we expect pool 1265 to be returned.
			// See test description above for details.
			expectedRoutePoolID: poolID1265Concentrated,
		},
		"cache is set to balancer - overwrites computed": {
			amountIn: defaultAmountInCache,

			preCachedRoutes: poolIDOneRoute,

			cacheOrderOfMagnitudeTokenIn: osmomath.OrderOfMagnitude(defaultAmountInCache.ToLegacyDec()),

			cacheExpiryDuration: time.Hour,

			// We expect balancer because it is cached.
			expectedRoutePoolID: poolIDOneBalancer,
		},
		"cache is set to balancer but for a different order of magnitude - computes new routes": {
			amountIn: defaultAmountInCache,

			preCachedRoutes: poolIDOneRoute,

			// Note that we multiply the order of magnitude by 10 so cache is not applied for this amount in.
			cacheOrderOfMagnitudeTokenIn: osmomath.OrderOfMagnitude(defaultAmountInCache.ToLegacyDec().MulInt64(10)),

			cacheExpiryDuration: time.Hour,

			// We expect pool 1265 because the cache is not applied.
			expectedRoutePoolID: poolID1265Concentrated,
		},
		"cache is expired - overwrites computed": {
			amountIn: defaultAmountInCache,

			preCachedRoutes: poolIDOneRoute,

			cacheOrderOfMagnitudeTokenIn: osmomath.OrderOfMagnitude(defaultAmountInCache.ToLegacyDec()),

			// Note: we rely on the fact that the it takes more than 1 nanosecond from the test set up to
			// test execution.
			cacheExpiryDuration: time.Nanosecond,

			// We expect pool 1265 because the cache with balancer pool expires.
			expectedRoutePoolID: poolID1265Concentrated,
		},
		"cache is not set, overwrites set, routes taken from overwrites (not computed)": {
			amountIn: defaultAmountInCache,

			overwriteRoutes: poolIDOneRoute,

			// For the default amount in, we expect pool 1265 to be returned.
			// However, we overwrite the routes with pool of ID 1.
			expectedRoutePoolID: poolIDOneBalancer,
		},
		"cache is set, overwrites set, routes taken from overwrites (not computed and not cache)": {
			amountIn: defaultAmountInCache,

			overwriteRoutes: poolIDOneRoute,

			preCachedRoutes: poolID1135Route,

			// For the default amount in, we expect pool 1265 to be returned.
			// However, we overwrite the routes (or cache) with pool of ID 1.
			expectedRoutePoolID: poolIDOneBalancer,
		},
	}

	for name, tc := range tests {
		tc := tc
		s.Run(name, func() {
			// Setup router config
			config := defaultRouterConfig
			// Note that we set one max route for ease of testing caching specifically.
			config.MaxRoutes = 1

			// Setup mainnet router
			router, tickMap, takerFeeMap := s.setupMainnetRouter(config)

			rankedRouteCache := cache.New()
			routesOverwrite := cache.NewRoutesOverwrite()

			if len(tc.preCachedRoutes.Routes) > 0 {
				rankedRouteCache.Set(usecase.FormatRankedRouteCacheKey(defaultTokenInDenom, defaultTokenOutDenom, tc.cacheOrderOfMagnitudeTokenIn), tc.preCachedRoutes, tc.cacheExpiryDuration)
			}

			if len(tc.overwriteRoutes.Routes) > 0 {
				routesOverwrite.Set(usecase.FormatRouteCacheKey(defaultTokenInDenom, defaultTokenOutDenom), tc.overwriteRoutes)
			}

			// Mock router use case.
			routerUsecase, _ := s.setupRouterAndPoolsUsecase(router, tickMap, takerFeeMap, rankedRouteCache, routesOverwrite)

			// System under test
			quote, err := routerUsecase.GetOptimalQuote(context.Background(), sdk.NewCoin(defaultTokenInDenom, tc.amountIn), defaultTokenOutDenom)

			// We only validate that error does not occur without actually validating the quote.
			s.Require().NoError(err)

			// By construction, this test always expects 1 route
			quoteRoutes := quote.GetRoute()
			s.Require().Len(quoteRoutes, 1)

			// By construction, this test always expects 1 pool
			routePools := quoteRoutes[0].GetPools()
			s.Require().Len(routePools, 1)

			// Validate that the pool ID is the expected one
			s.Require().Equal(tc.expectedRoutePoolID, routePools[0].GetId())

			// Validate that the quote is not nil
			s.Require().NotNil(quote.GetAmountOut())

		})
	}
}

// Basic happy path test for OverwriteRoutes and LoadOverwriteRoutes.
//
// Similar to TestGetOptimalQuote_Cache_Overwrites, this test is set up by focusing on ATOM / OSMO mainnet state pool.
// We restrict the number of routes via config.
//
// As of today there are 3 major ATOM / OSMO pools:
// Pool ID 1: https://app.osmosis.zone/pool/1 (balancer) 0.2% spread factor and 20M of liquidity to date
// Pool ID 1135: https://app.osmosis.zone/pool/1135 (concentrated) 0.2% spread factor and 14M of liquidity to date
// Pool ID 1265: https://app.osmosis.zone/pool/1265 (concentrated) 0.05% spread factor and 224K of liquidity to date
func (s *RouterTestSuite) TestOverwriteRoutes_LoadOverwriteRoutes() {
	const tempPath = "temp"

	s.Setup()

	// Setup router config
	config := defaultRouterConfig
	// Note that we set one max route for ease of testing caching specifically.
	config.MaxRoutes = 1

	// Setup mainnet router
	router, tickMap, takerFeeMap := s.setupMainnetRouter(config)

	// Mock router use case.
	routerUsecase, _ := s.setupRouterAndPoolsUsecase(router, tickMap, takerFeeMap, cache.New(), cache.NewRoutesOverwrite())
	routerUsecase = usecase.WithOverwriteRoutesPath(routerUsecase, tempPath)

	// Without overwrite this is the pool ID we expect given the amount in.
	s.validatePoolIDInRoute(routerUsecase, sdk.NewCoin(UOSMO, defaultAmountInCache), ATOM, poolID1265Concentrated)

	defer func() {
		// Clean up
		os.RemoveAll(tempPath)
	}()

	// System under test #1
	err := routerUsecase.OverwriteRoutes(context.Background(), UOSMO, poolIDOneRoute.Routes)
	s.Require().NoError(err)

	// With overwrite this is the pool ID we expect given the amount in.
	s.validatePoolIDInRoute(routerUsecase, sdk.NewCoin(UOSMO, defaultAmountInCache), ATOM, poolIDOneBalancer)

	// Validate that the overwrite can be modified
	// System under test #2
	err = routerUsecase.OverwriteRoutes(context.Background(), UOSMO, poolID1135Route.Routes)
	s.Require().NoError(err)

	// With overwrite this is the pool ID we expect given the amount in.
	s.validatePoolIDInRoute(routerUsecase, sdk.NewCoin(UOSMO, defaultAmountInCache), ATOM, poolID1135Concentrated)

	// Now, drop the original use case and create a new one
	routerUsecase, _ = s.setupRouterAndPoolsUsecase(router, tickMap, takerFeeMap, cache.New(), cache.NewRoutesOverwrite())
	routerUsecase = usecase.WithOverwriteRoutesPath(routerUsecase, tempPath)

	// 	// Without overwrite this is the pool ID we expect given the amount in.
	s.validatePoolIDInRoute(routerUsecase, sdk.NewCoin(UOSMO, defaultAmountInCache), ATOM, poolID1265Concentrated)

	// Load overwrite
	err = routerUsecase.LoadOverwriteRoutes(context.Background())
	s.Require().NoError(err)

	// With overwrite this is the pool ID we expect given the amount in.
	s.validatePoolIDInRoute(routerUsecase, sdk.NewCoin(UOSMO, defaultAmountInCache), ATOM, poolID1135Concentrated)
}

// validates that for the given coinIn and tokenOutDenom, there is one route with one pool ID equal to the expectedPoolID.
// This helper is useful in specific tests that rely on this configuration.
func (s *RouterTestSuite) validatePoolIDInRoute(routerUseCase mvc.RouterUsecase, coinIn sdk.Coin, tokenOutDenom string, expectedPoolID uint64) {
	// Get quote
	quote, err := routerUseCase.GetOptimalQuote(context.Background(), coinIn, tokenOutDenom)
	s.Require().NoError(err)

	quoteRoutes := quote.GetRoute()
	s.Require().Len(quoteRoutes, 1)

	routePools := quoteRoutes[0].GetPools()
	s.Require().Len(routePools, 1)

	// Validate that the pool ID is the expected one
	s.Require().Equal(expectedPoolID, routePools[0].GetId())
}
