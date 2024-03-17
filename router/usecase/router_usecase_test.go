package usecase_test

import (
	"context"
	"fmt"
	"os"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/viper"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
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
	poolID1400Concentrated = uint64(1400)
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

	defaultPricingRouterConfig = routertesting.DefaultPricingRouterConfig
	defaultPricingConfig       = routertesting.DefaultPricingConfig
)

// Tests the call to handleRoutes by mocking the router repository and pools use case
// with relevant data.
func (s *RouterTestSuite) TestHandleRoutes() {
	const (
		defaultTimeoutDuration = time.Second * 10

		tokenInDenom  = "uosmo"
		tokenOutDenom = "uion"

		minOsmoLiquidity = 100
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
			TotalValueLockedUSDC: osmomath.NewInt(int64(minOsmoLiquidity*usecase.OsmoPrecisionMultiplier + 1)),
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
				// No need to set taker fees on the mock since they are only relevant when
				// set on the router for this test.
			}

			candidateRouteCache := cache.New()
			candidateRouteCache.Set(usecase.FormatCandidateRouteCacheKey(tokenInDenom, tokenOutDenom), tc.repositoryRoutes, time.Hour)

			poolsUseCaseMock := &mocks.PoolsUsecaseMock{
				// These are the pools returned by the call to GetAllPools
				Pools: tc.repositoryPools,
			}

			routerUseCase := usecase.NewRouterUsecase(defaultTimeoutDuration, routerRepositoryMock, poolsUseCaseMock, domain.RouterConfig{
				RouteCacheEnabled: !tc.isCacheDisabled,
			}, emptyCosmWasmPoolsRouterConfig, &log.NoOpLogger{}, cache.New(), candidateRouteCache)

			routerUseCaseImpl, ok := routerUseCase.(*usecase.RouterUseCaseImpl)
			s.Require().True(ok)

			// Initialize router
			router := usecase.NewRouter(defaultRouterConfig, emptyCosmWasmPoolsRouterConfig, &log.NoOpLogger{})
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

			cachedCandidateRoutes, err := routerUseCaseImpl.GetCachedCandidateRoutes(ctx, tokenInDenom, tokenOutDenom)
			// For the case where the cache is disabled, the expected routes in cache
			// will be the same as the original routes in the repository.
			// Check that router repository was updated
			s.Require().Equal(tc.expectedCandidateRoutes, cachedCandidateRoutes)
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
// Pool ID 1: https://app.osmosis.zone/pool/1 (balancer) 0.2% spread factor and 18M of liquidity to date
// Pool ID 1135: https://app.osmosis.zone/pool/1135 (concentrated) 0.2% spread factor and 10M of liquidity to date
// Pool ID 1265: https://app.osmosis.zone/pool/1265 (concentrated) 0.05% spread factor and 1.4M of liquidity to date
// Pool ID 1265: https://app.osmosis.zone/pool/1400 (concentrated) 0.00% spread factor and 322K of liquidity to date
//
// Based on this state, the small amounts of token in should go through pool 1265
// Medium amounts of token in should go through pool 1135
// and large amounts of token in should go through pool 1.
//
// For the purposes of testing cache, we focus on a small amount of token in (1_000_000 uosmo), expecting pool 1265 to be returned.
// We will, however, tweak the cache by test case to force other pools to be returned and ensure that the cache is used.
func (s *RouterTestSuite) TestGetOptimalQuote_Cache_Overwrites() {
	var (
		defaultTokenInDenom  = UOSMO
		defaultTokenOutDenom = ATOM
	)

	tests := map[string]struct {
		preCachedCandidateRoutes sqsdomain.CandidateRoutes

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

			preCachedCandidateRoutes: poolIDOneRoute,

			cacheExpiryDuration: time.Hour,

			// We expect balancer because it is cached.
			expectedRoutePoolID: poolIDOneBalancer,
		},
		"cache is expired - overwrites computed": {
			amountIn: defaultAmountInCache,

			preCachedCandidateRoutes: poolIDOneRoute,

			// Note: we rely on the fact that the it takes more than 1 nanosecond from the test set up to
			// test execution.
			cacheExpiryDuration: time.Nanosecond,

			// We expect pool 1265 because the cache with balancer pool expires.
			expectedRoutePoolID: poolID1265Concentrated,
		},
	}

	for name, tc := range tests {
		tc := tc
		s.Run(name, func() {
			// Setup mainnet router
			router, mainnetState := s.SetupDefaultMainnetRouter()

			rankedRouteCache := cache.New()
			candidateRouteCache := cache.New()

			if len(tc.preCachedCandidateRoutes.Routes) > 0 {
				candidateRouteCache.Set(usecase.FormatCandidateRouteCacheKey(defaultTokenInDenom, defaultTokenOutDenom), tc.preCachedCandidateRoutes, tc.cacheExpiryDuration)
			}

			// Mock router use case.
			mainnetUseCase := s.SetupRouterAndPoolsUsecase(router, mainnetState, rankedRouteCache, candidateRouteCache)

			// System under test
			quote, err := mainnetUseCase.Router.GetOptimalQuote(context.Background(), sdk.NewCoin(defaultTokenInDenom, tc.amountIn), defaultTokenOutDenom)

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

// This test validates that routes can be found for all supported tokens.
// Fails if not.
// We use this test in CI for detecting tokens with unsupported pricing.
// The config used is the `config.json` in root which is expected to be as close
// to mainnet as possible.
//
// The mainnet state must be manually updated when needed with 'make sqs-update-mainnet-state'
func (s *RouterTestSuite) TestGetCandidateRoutes_Chain_FindUnsupportedRoutes() {
	env := os.Getenv("CI_SQS_ROUTE_TEST")
	if env != "true" {
		s.T().Skip("This test exists to identify which mainnet routes are unsupported")
	}

	viper.SetConfigFile("../../config.json")
	err := viper.ReadInConfig()
	s.Require().NoError(err)

	// Unmarshal the config into your Config struct
	var config domain.Config
	err = viper.Unmarshal(&config)
	s.Require().NoError(err)

	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(*config.Router)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata(context.Background())
	s.Require().NoError(err)

	fmt.Println("Tokens with no routes when min osmo liquidity is non-zero:")

	errorCounter := 0
	zeroPriceCounterMinLiq := 0
	s.Require().NotZero(len(tokenMetadata))
	for chainDenom, tokenMeta := range tokenMetadata {

		routes, err := router.GetCandidateRoutes(chainDenom, USDC)
		if err != nil {
			fmt.Printf("Error for %s  -- %s\n", chainDenom, tokenMeta.HumanDenom)
			errorCounter++
			continue
		}

		if len(routes.Routes) == 0 {
			fmt.Printf("No route for %s  -- %s\n", chainDenom, tokenMeta.HumanDenom)
			zeroPriceCounterMinLiq++
			continue
		}
	}

	s.Require().Zero(errorCounter)

	// Print space
	fmt.Printf("\n\n\n")
	fmt.Println("Tokens with no routes even when min osmo liquidity is set to zero:")

	zeroPriceCounterNoMinLiq := 0
	// Now set min osmo liquidity to zero to identify which tokens are missing prices even when we
	// don't have liquidity filtering.
	config.Router.MinOSMOLiquidity = 0
	// Set up mainnet mock state.
	router, mainnetState = s.SetupMainnetRouter(*config.Router)
	mainnetUsecase = s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())
	for chainDenom, tokenMeta := range tokenMetadata {

		routes, err := router.GetCandidateRoutes(chainDenom, USDC)
		if err != nil {
			fmt.Printf("Error for %s  -- %s\n", chainDenom, tokenMeta.HumanDenom)
			errorCounter++
			continue
		}

		if len(routes.Routes) == 0 {
			fmt.Printf("No route for %s  -- %s (no min liq filtering)\n", chainDenom, tokenMeta.HumanDenom)
			zeroPriceCounterNoMinLiq++
			continue
		}
	}

	s.Require().Zero(errorCounter)

	s.Zero(zeroPriceCounterMinLiq)
	s.Require().Zero(zeroPriceCounterNoMinLiq, "There are tokens with no routes even when min osmo liquidity is set to zero")
}

// We use this test as a way to ensure that we multiply the amount in by the route fraction.
// We caught a bug in production where for WBTC -> USDC swap the price impact was excessively large.
// The reason ended up being using a total amount for estimating the execution price.
// We keep this test to ensure that we don't regress on this.
// In the future, we should have stricter unit tests for this.
func (s *RouterTestSuite) TestPriceImpactRoute_Fractions() {
	viper.SetConfigFile("../../config.json")
	err := viper.ReadInConfig()
	s.Require().NoError(err)

	// Unmarshal the config into your Config struct
	var config domain.Config
	err = viper.Unmarshal(&config)
	s.Require().NoError(err)

	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(*config.Router)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata(context.Background())

	chainWBTC, err := mainnetUsecase.Tokens.GetChainDenom(context.Background(), "wbtc")
	s.Require().NoError(err)

	wbtcMetadata, ok := tokenMetadata[chainWBTC]
	s.Require().True(ok)

	// Get quote.
	quote, err := mainnetUsecase.Router.GetOptimalQuote(context.Background(), sdk.NewCoin(chainWBTC, osmomath.NewInt(1_00_000_000)), USDC)
	s.Require().NoError(err)

	// Prepare quote result.
	_, _, err = quote.PrepareResult(context.Background(), osmomath.NewDec(int64(wbtcMetadata.Precision)))

	priceImpact := quote.GetPriceImpact()

	// 0.07 is chosen arbitrarily with extra buffer because we update test mainnet state frequently and
	// would like to avoid flakiness.
	s.Require().True(priceImpact.LT(osmomath.MustNewDecFromStr("0.07")))
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
