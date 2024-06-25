package usecase_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/viper"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	poolsusecase "github.com/osmosis-labs/sqs/pools/usecase"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v25/x/gamm/pool-models/balancer"
)

const (
	// See description of:
	// - TestGetOptimalQuote_Cache_Overwrites
	// - TestOverwriteRoutes
	// for details.
	poolIDOneBalancer      = uint64(1)
	poolID1135Concentrated = uint64(1135)
	poolID1265Concentrated = uint64(1265)
	poolID1399Concentrated = uint64(1399)
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

	defaultRouterConfig        = routertesting.DefaultRouterConfig
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

		minPoolLiquidityCap = 100
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
			PoolLiquidityCap: osmomath.NewInt(int64(minPoolLiquidityCap*OsmoPrecisionMultiplier + 1)),
			PoolDenoms:       []string{tokenInDenom, tokenOutDenom},
			Balances:         balancerCoins,
			SpreadFactor:     DefaultSpreadFactor,
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
			PreferredPoolIDs:    []uint64{},
			MinPoolLiquidityCap: minPoolLiquidityCap,
		}
	)

	testCases := []struct {
		name string

		repositoryRoutes     sqsdomain.CandidateRoutes
		repositoryPools      []sqsdomain.PoolI
		takerFeeMap          sqsdomain.TakerFeeMap
		isCacheDisabled      bool
		shouldSkipAddToCache bool

		expectedCandidateRoutes sqsdomain.CandidateRoutes

		expectedError    error
		expectedIsCached bool
	}{
		{
			name: "routes in cache -> use them",

			repositoryRoutes: singleDefaultRoutes,
			repositoryPools:  emptyPools,

			expectedCandidateRoutes: singleDefaultRoutes,
			expectedIsCached:        true,
		},
		{
			name: "cache is disabled in config -> recomputes routes despite having available in cache",

			repositoryRoutes: singleDefaultRoutes,
			repositoryPools:  emptyPools,
			isCacheDisabled:  true,

			expectedCandidateRoutes: emptyRoutes,
			expectedIsCached:        false,
		},
		{
			name: "no routes in cache but relevant pools in store -> recomputes routes & caches them",

			repositoryRoutes:     emptyRoutes,
			repositoryPools:      defaultSinglePools,
			shouldSkipAddToCache: true,

			expectedCandidateRoutes: singleDefaultRoutes,
			expectedIsCached:        true,
		},
		{
			name: "empty routes in cache but relevant pools in store -> does not recompute routes",

			repositoryRoutes: emptyRoutes,
			repositoryPools:  defaultSinglePools,

			expectedCandidateRoutes: emptyRoutes,
			expectedIsCached:        true,
		},
		{
			name: "no routes in cache and no relevant pools in store -> returns no routes & caches them",

			repositoryRoutes: emptyRoutes,
			repositoryPools:  emptyPools,

			expectedCandidateRoutes: emptyRoutes,
			expectedIsCached:        true,
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

			routerRepositoryMock := routerrepo.New()

			candidateRouteCache := cache.New()

			if !tc.shouldSkipAddToCache {
				candidateRouteCache.Set(usecase.FormatCandidateRouteCacheKey(tokenInDenom, tokenOutDenom), tc.repositoryRoutes, time.Hour)
			}

			poolsUseCaseMock := &mocks.PoolsUsecaseMock{
				// These are the pools returned by the call to GetAllPools
				Pools: tc.repositoryPools,
			}

			routerUseCase := usecase.NewRouterUsecase(routerRepositoryMock, poolsUseCaseMock, domain.RouterConfig{
				RouteCacheEnabled: !tc.isCacheDisabled,
			}, emptyCosmWasmPoolsRouterConfig, &log.NoOpLogger{}, cache.New(), candidateRouteCache)

			// Validate and sort pools
			sortedPools := usecase.ValidateAndSortPools(tc.repositoryPools, emptyCosmWasmPoolsRouterConfig, []uint64{}, noOpLogger)

			// Filter pools by min liquidity
			sortedPools = usecase.FilterPoolsByMinLiquidity(sortedPools, minPoolLiquidityCap)

			routerUseCaseImpl, ok := routerUseCase.(*usecase.RouterUseCaseImpl)
			s.Require().True(ok)

			// System under test
			ctx := context.Background()
			// TODO: filter pools per router config
			actualCandidateRoutes, err := routerUseCaseImpl.HandleRoutes(ctx, sortedPools, sdk.NewCoin(tokenInDenom, one), tokenOutDenom, defaultRouterConfig.MaxRoutes, defaultRouterConfig.MaxPoolsPerRoute)

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

			cachedCandidateRoutes, isCached, err := routerUseCaseImpl.GetCachedCandidateRoutes(ctx, tokenInDenom, tokenOutDenom)
			// For the case where the cache is disabled, the expected routes in cache
			// will be the same as the original routes in the repository.
			// Check that router repository was updated
			s.Require().Equal(tc.expectedCandidateRoutes, cachedCandidateRoutes)
			s.Require().Equal(tc.expectedIsCached, isCached)
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

		defaultSingleRoute = WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
			deafaultPool,
		})

		alloyRouteOne = WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
			&mocks.MockRoutablePool{ID: defaultPoolID, SQSPoolType: domain.AlloyedTransmuter},
			&mocks.MockRoutablePool{ID: defaultPoolID + 1, SQSPoolType: domain.Balancer},
		})

		alloyRouteTwo = WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
			&mocks.MockRoutablePool{ID: defaultPoolID, SQSPoolType: domain.AlloyedTransmuter},
			&mocks.MockRoutablePool{ID: defaultPoolID + 2, SQSPoolType: domain.StableSwap},
		})

		transmuterRouteOne = WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
			&mocks.MockRoutablePool{ID: defaultPoolID + 1, SQSPoolType: domain.Balancer},
			&mocks.MockRoutablePool{ID: defaultPoolID, SQSPoolType: domain.TransmuterV1},
		})

		transmuterRouteTwo = WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
			&mocks.MockRoutablePool{ID: defaultPoolID + 2, SQSPoolType: domain.StableSwap},
			&mocks.MockRoutablePool{ID: defaultPoolID, SQSPoolType: domain.TransmuterV1},
		})

		alloyTransmuterV1Route = WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
			&mocks.MockRoutablePool{ID: defaultPoolID, SQSPoolType: domain.AlloyedTransmuter},
			&mocks.MockRoutablePool{ID: defaultPoolID + 1, SQSPoolType: domain.TransmuterV1},
		})
	)

	wrapRoute := func(r route.RouteImpl) usecase.RouteWithOutAmount {
		return usecase.RouteWithOutAmount{
			RouteImpl: r,
			// Note: amount is not relevant for this test
		}
	}

	tests := map[string]struct {
		routes []usecase.RouteWithOutAmount

		expectedRoutes []route.RouteImpl
	}{
		"empty routes": {
			routes:         []usecase.RouteWithOutAmount{},
			expectedRoutes: []route.RouteImpl{},
		},

		"single route single pool": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(defaultSingleRoute),
			},

			expectedRoutes: []route.RouteImpl{
				defaultSingleRoute,
			},
		},

		"single route two different pools": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					deafaultPool,
					otherPool,
				})),
			},

			expectedRoutes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					deafaultPool,
					otherPool,
				}),
			},
		},

		// Note that filtering only happens if pool ID duplciated across different routes.
		// Duplicate pool IDs within the same route are filtered out at a different step
		// in the router logic.
		"single route two same pools (have no effect on filtering)": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					deafaultPool,
					deafaultPool,
				})),
			},

			expectedRoutes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					deafaultPool,
					deafaultPool,
				}),
			},
		},

		"two single hop routes and no duplicates": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(defaultSingleRoute),

				wrapRoute(WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					otherPool,
				})),
			},

			expectedRoutes: []route.RouteImpl{
				defaultSingleRoute,

				WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					otherPool,
				}),
			},
		},

		"two single hop routes with duplicates (second filtered)": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(defaultSingleRoute),

				wrapRoute(defaultSingleRoute),
			},

			expectedRoutes: []route.RouteImpl{
				defaultSingleRoute,
			},
		},

		"three route. first and second overlap. second and third overlap. second is filtered out but not third": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(defaultSingleRoute),

				wrapRoute(WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					deafaultPool, // first and second overlap
					otherPool,    // second and third overlap
				})),

				wrapRoute(WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					otherPool,
				})),
			},

			expectedRoutes: []route.RouteImpl{
				defaultSingleRoute,

				WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					otherPool,
				}),
			},
		},

		"two routes with duplicate alloy -> not filtered": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(alloyRouteOne),

				wrapRoute(alloyRouteTwo),
			},

			expectedRoutes: []route.RouteImpl{
				alloyRouteOne,

				alloyRouteTwo,
			},
		},

		"two routes with duplicate transmuter -> not filtered": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(transmuterRouteOne),

				wrapRoute(transmuterRouteTwo),
			},

			expectedRoutes: []route.RouteImpl{
				transmuterRouteOne,

				transmuterRouteTwo,
			},
		},

		"two exact routes with alloy and transmuter -> filtered": {
			routes: []usecase.RouteWithOutAmount{
				wrapRoute(alloyTransmuterV1Route),
				wrapRoute(alloyTransmuterV1Route),
			},

			expectedRoutes: []route.RouteImpl{
				alloyTransmuterV1Route,
				alloyTransmuterV1Route,
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
				WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
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
				WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
					mocks.WithPoolID(mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), &balancer.Pool{}), defaultPoolID),
				}),
				WithRoutePools(route.RouteImpl{}, []domain.RoutablePool{
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
// Pool ID 1: https://app.osmosis.zone/pool/1 (balancer) 0.2% spread factor and 8M of liquidity to date
// Pool ID 1135: https://app.osmosis.zone/pool/1135 (concentrated) 0.2% spread factor and 4M of liquidity to date
// Pool ID 1265: https://app.osmosis.zone/pool/1265 (concentrated) 0.05% spread factor and 268K of liquidity to date
// Pool ID 1399: https://app.osmosis.zone/pool/1399 (concentrated) 0.01% spread factor and 75K of liquidity to date
// Pool ID 1400: https://app.osmosis.zone/pool/1400 (concentrated) 0.00% spread factor and 149K of liquidity to date
//
// Based on this state, the small amounts of token in should go through pool 1265
// Medium amounts of token in should go through pool 1135
// and large amounts of token in should go through pool 1135.
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

			// For the default amount in, we expect this pool to be returned.
			// See test description above for details.
			expectedRoutePoolID: poolID1400Concentrated,
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

			// We expect this pool because the cache with balancer pool expires.
			expectedRoutePoolID: poolID1400Concentrated,
		},
	}

	for name, tc := range tests {
		tc := tc
		s.Run(name, func() {
			// Setup mainnet router
			mainnetState := s.SetupMainnetState()

			rankedRouteCache := cache.New()
			candidateRouteCache := cache.New()

			if len(tc.preCachedCandidateRoutes.Routes) > 0 {
				candidateRouteCache.Set(usecase.FormatCandidateRouteCacheKey(defaultTokenInDenom, defaultTokenOutDenom), tc.preCachedCandidateRoutes, tc.cacheExpiryDuration)
			}

			// Mock router use case.
			mainnetUseCase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRankedRoutesCache(rankedRouteCache), routertesting.WithCandidateRoutesCache(candidateRouteCache))

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

	const (
		// This was selected by looking at the routes and concluding that it's
		// probably fine. Might need to re-evaluate in the future.
		expectedZeroPoolCount = 37
	)

	viper.SetConfigFile("../../config.json")
	err := viper.ReadInConfig()
	s.Require().NoError(err)

	// Unmarshal the config into your Config struct
	var config domain.Config
	err = viper.Unmarshal(&config)
	s.Require().NoError(err)

	// Set up mainnet mock state.
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(*config.Router), routertesting.WithPricingConfig(*config.Pricing))

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata()
	s.Require().NoError(err)

	fmt.Println("Tokens with no routes when min osmo liquidity is non-zero:")

	one := osmomath.OneInt()

	errorCounter := 0
	zeroRouteCount := 0
	s.Require().NotZero(len(tokenMetadata))
	for chainDenom, tokenMeta := range tokenMetadata {

		routes, err := usecase.GetCandidateRoutes(mainnetState.Pools, sdk.NewCoin(chainDenom, one), USDC, config.Router.MaxRoutes, config.Router.MaxPoolsPerRoute, noOpLogger)
		if err != nil {
			fmt.Printf("Error for %s  -- %s\n", chainDenom, tokenMeta.HumanDenom)
			errorCounter++
			continue
		}

		if len(routes.Routes) == 0 {
			fmt.Printf("No route for %s  -- %s\n", chainDenom, tokenMeta.HumanDenom)
			zeroRouteCount++
			continue
		}
	}

	s.Require().Zero(errorCounter)

	// Print space
	fmt.Printf("\n\n\n")
	fmt.Println("Tokens with no routes even when min osmo liquidity is set to zero:")

	zeroPriceCounterNoMinLiq := 0
	// Now set min liquidity capitalization to zero to identify which tokens are missing prices even when we
	// don't have liquidity filtering.
	config.Router.MinPoolLiquidityCap = 0
	// Set up mainnet mock state.
	mainnetState = s.SetupMainnetState()
	mainnetUsecase = s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(*config.Router), routertesting.WithPricingConfig(*config.Pricing))

	for chainDenom, tokenMeta := range tokenMetadata {

		routes, err := usecase.GetCandidateRoutes(mainnetState.Pools, sdk.NewCoin(chainDenom, one), USDC, config.Router.MaxRoutes, config.Router.MaxPoolsPerRoute, noOpLogger)
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

	// Note that if we update test state, these are likely to change
	s.Require().Equal(expectedZeroPoolCount, zeroRouteCount)
	s.Require().Equal(expectedZeroPoolCount, zeroPriceCounterNoMinLiq, "There are tokens with no routes even when min osmo liquidity is set to zero")
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
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(*config.Router), routertesting.WithPricingConfig(*config.Pricing), routertesting.WithRouterConfig(*config.Router), routertesting.WithPricingConfig(*config.Pricing))

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata()

	chainWBTC, err := mainnetUsecase.Tokens.GetChainDenom("wbtc")
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

// This is a sanity-check to ensure that the pools are sorted as intended and persisted
// in the router usecase state.
func (s *RouterTestSuite) TestSortPools() {
	const (
		// the minimum number of pools should  only change if liqudiity falls below MinPoolLiquidityCap. As a result
		// this is a good high-level check to ensure that the pools are being loaded correctly.
		expectedMinNumPools = 239

		// If mainnet state is updated
		expectedTopPoolID = uint64(1283)
	)

	mainnetState := s.SetupMainnetState()

	mainnetUseCase := s.SetupRouterAndPoolsUsecase(mainnetState)

	pools, err := mainnetUseCase.Pools.GetAllPools()
	s.Require().NoError(err)

	// Validate and sort pools
	sortedPools := usecase.ValidateAndSortPools(pools, emptyCosmWasmPoolsRouterConfig, []uint64{}, noOpLogger)

	// Filter pools by min liquidity
	sortedPools = usecase.FilterPoolsByMinLiquidity(sortedPools, defaultRouterConfig.MinPoolLiquidityCap)

	s.Require().GreaterOrEqual(len(sortedPools), expectedMinNumPools)

	// Check that the top pool is the expected one.
	s.Require().Equal(expectedTopPoolID, sortedPools[0].GetId())
}

// Validates ConvertMinTokensPoolLiquidityCapToFilter method per its spec.
func (s *RouterTestSuite) TestConvertMinTokensPoolLiquidityCapToFilter() {
	var (
		defaultFilters = routertesting.DefaultRouterConfig.DynamicMinLiquidityCapFiltersDesc

		defaultConfigFilter = routertesting.DefaultRouterConfig.MinPoolLiquidityCap

		defaultThresholdMinPoolLiquidityCap = defaultFilters[0].MinTokensCap

		defaultAboveThresholdFilterValue = defaultFilters[0].FilterValue
	)

	tests := []struct {
		name string

		minLiqCapFilterEntries []domain.DynamicMinLiquidityCapFilterEntry

		minTokensPoolLiquidityCap uint64

		expectedFilter uint64
	}{
		{
			name: "min pool liquidity cap at threshold -> return dynamic filter value",

			minLiqCapFilterEntries: defaultFilters,

			minTokensPoolLiquidityCap: defaultThresholdMinPoolLiquidityCap,

			expectedFilter: defaultAboveThresholdFilterValue,
		},

		{
			name: "min pool liquidity cap above threshold -> return dynamic filter value",

			minLiqCapFilterEntries: defaultFilters,

			minTokensPoolLiquidityCap: defaultThresholdMinPoolLiquidityCap + 1,

			expectedFilter: defaultAboveThresholdFilterValue,
		},

		{
			name: "min pool liquidity cap below threshold -> return default filter value",

			minLiqCapFilterEntries: defaultFilters,

			minTokensPoolLiquidityCap: defaultThresholdMinPoolLiquidityCap - 1,

			expectedFilter: defaultConfigFilter,
		},

		{
			name: "empty filters -> return default filter value",

			minLiqCapFilterEntries: []domain.DynamicMinLiquidityCapFilterEntry{},

			minTokensPoolLiquidityCap: defaultThresholdMinPoolLiquidityCap - 1,

			expectedFilter: defaultConfigFilter,
		},
		{
			name: "multiple pre-configured filters -> choice falls in-between",

			minLiqCapFilterEntries: []domain.DynamicMinLiquidityCapFilterEntry{
				{
					MinTokensCap: 300_000,
					FilterValue:  30_000,
				},
				{
					MinTokensCap: 20_000,
					FilterValue:  2_000,
				},
				{
					MinTokensCap: 1_000,
					FilterValue:  100,
				},
			},

			// Above 1_000 and below 20_000.
			minTokensPoolLiquidityCap: 5000,

			expectedFilter: 100,
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			// Set up mainnet mock state.
			mainnetState := s.SetupMainnetState()

			config := routertesting.DefaultRouterConfig
			config.DynamicMinLiquidityCapFiltersDesc = tt.minLiqCapFilterEntries

			mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(config))

			// System under test
			actualFilter := mainnetUsecase.Router.ConvertMinTokensPoolLiquidityCapToFilter(tt.minTokensPoolLiquidityCap)

			// Validate result.
			s.Require().Equal(tt.expectedFilter, actualFilter)
		})
	}
}

// This test runs tests against GetCustomDirectQuotes to ensure that the method correctly calculates
// quote across multi pool route.
func (s *RouterTestSuite) TestGetCustomQuote_GetCustomDirectQuotes_Mainnet_UOSMOUSDC() {
	config := routertesting.DefaultRouterConfig
	config.MaxPoolsPerRoute = 5
	config.MaxRoutes = 10

	var (
		amountIn = osmomath.NewInt(5000000)
	)

	mainnetState := s.SetupMainnetState()

	// Setup router repository mock
	routerRepositoryMock := routerrepo.New()
	routerRepositoryMock.SetTakerFees(mainnetState.TakerFeeMap)

	// Setup pools usecase mock.
	poolsUsecase := poolsusecase.NewPoolsUsecase(&domain.PoolsConfig{}, "node-uri-placeholder", routerRepositoryMock, domain.UnsetScalingFactorGetterCb)
	poolsUsecase.StorePools(mainnetState.Pools)

	routerUsecase := usecase.NewRouterUsecase(routerRepositoryMock, poolsUsecase, config, emptyCosmWasmPoolsRouterConfig, &log.NoOpLogger{}, cache.New(), cache.New())

	// Test cases
	testCases := []struct {
		// test name
		name string

		// token being swapped
		tokenIn sdk.Coin

		// token to be received
		tokenOutDenom []string

		// pools route path for swap
		poolID []uint64

		// usually it's the number of pools given,
		// unless any of those pools does not have given asset pair.
		expectedNumOfRoutes int

		// for single-hop it matches poolID slice
		expectedPoolID []uint64

		err error
	}{
		{
			name:          "Fail: empty tokenOutDenom",
			tokenIn:       sdk.NewCoin(UOSMO, amountIn),
			tokenOutDenom: []string{},
			poolID: []uint64{
				1, // OSMO - ATOM
			},
			err: usecase.ErrValidationFailed,
		},
		{
			name:          "Fail: empty poolID",
			tokenIn:       sdk.NewCoin(UOSMO, amountIn),
			tokenOutDenom: []string{ATOM},
			poolID:        []uint64{},
			err:           usecase.ErrValidationFailed,
		},
		{
			name:          "Fail: mismatch poolID and tokenOutDenom",
			tokenIn:       sdk.NewCoin(UOSMO, amountIn),
			tokenOutDenom: []string{ATOM},
			poolID:        []uint64{1, 2},
			err:           usecase.ErrValidationFailed,
		},
		{
			name:          "Single pool: OSMO-ATOM - happy case",
			tokenIn:       sdk.NewCoin(UOSMO, amountIn),
			tokenOutDenom: []string{ATOM},
			poolID: []uint64{
				1, // OSMO - ATOM
			},
			expectedNumOfRoutes: 1,
			expectedPoolID:      []uint64{1},
		},
		{
			name:          "Single pool: OSMO-ATOM - fail case: out denom not found",
			tokenIn:       sdk.NewCoin(UOSMO, amountIn),
			tokenOutDenom: []string{ATOM},
			poolID: []uint64{
				1093, // OSMO - AKT
			},
			err: usecase.ErrTokenOutDenomPoolNotFound,
		},
		{
			name:          "Single pool: ATOM-OSMO - fail case: in denom not found",
			tokenIn:       sdk.NewCoin(ATOM, amountIn),
			tokenOutDenom: []string{UOSMO},
			poolID: []uint64{
				1480, // AKT - USDC
			},
			err: usecase.ErrTokenInDenomPoolNotFound,
		},
		{
			name:          "Multi pool: OSMO-USDC - happy case",
			tokenIn:       sdk.NewCoin(UOSMO, amountIn),
			tokenOutDenom: []string{AKT, USDC},
			poolID: []uint64{
				1093, // OSMO - AKT
				1301, // AKT - USDC
			},
			expectedNumOfRoutes: 2,
			expectedPoolID:      []uint64{1093, 1301},
		},
		{
			name:          "Multi pool: OSMO-USDC - fail case",
			tokenIn:       sdk.NewCoin(UOSMO, amountIn),
			tokenOutDenom: []string{ATOM, USDT},
			poolID: []uint64{
				1,    // OSMO - ATOM
				1301, // AKT - USDC
			},
			expectedNumOfRoutes: 2,
			expectedPoolID:      []uint64{1093, 1301},
			err:                 usecase.ErrTokenInDenomPoolNotFound,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			quotes, err := routerUsecase.GetCustomDirectQuoteMultiPool(context.Background(), tc.tokenIn, tc.tokenOutDenom, tc.poolID)
			s.Require().ErrorIs(err, tc.err)
			if err != nil {
				return // nothing else to do
			}

			// token in must match
			s.Require().Equal(quotes.GetAmountIn().Denom, tc.tokenIn.Denom)
			s.Require().Equal(tc.expectedNumOfRoutes, len(quotes.GetRoute()))
			s.validateExpectedPoolIDMultiRouteOneHopQuote(quotes, tc.expectedPoolID)
		})
	}
}

func (s *RouterTestSuite) TestCutRoutesForSplits() {

	// Note: contents are irrelevant. Only count of routes matters for this test.
	var (
		defaultRoute = route.RouteImpl{}

		oneRoute    = []route.RouteImpl{defaultRoute}
		threeRoutes = []route.RouteImpl{defaultRoute, defaultRoute, defaultRoute}
	)

	testcases := []struct {
		name string

		maxSplitRoutes int
		routes         []route.RouteImpl

		expectedRoutesLen int
	}{
		{
			name: "empty routes & disable split routes for max split",

			routes:         []route.RouteImpl{},
			maxSplitRoutes: domain.DisableSplitRoutes,

			expectedRoutesLen: 0,
		},
		{
			name:           "empty routes & 1 max split",
			routes:         []route.RouteImpl{},
			maxSplitRoutes: 1,

			expectedRoutesLen: 0,
		},
		{
			name: "1 route & disable split routes for max split",

			routes:         oneRoute,
			maxSplitRoutes: domain.DisableSplitRoutes,

			expectedRoutesLen: 1,
		},
		{
			name:           "1 route & 1 max split",
			routes:         oneRoute,
			maxSplitRoutes: 1,

			expectedRoutesLen: 1,
		},
		{
			name:           "1 route & 2 max split",
			routes:         oneRoute,
			maxSplitRoutes: 2,

			expectedRoutesLen: 1,
		},
		{
			name:           "3 routes & disable split routes for max split",
			routes:         threeRoutes,
			maxSplitRoutes: domain.DisableSplitRoutes,

			expectedRoutesLen: 1,
		},
		{
			name:           "3 routes & 2 routes for max split",
			routes:         threeRoutes,
			maxSplitRoutes: 2,

			expectedRoutesLen: 2,
		},
		{
			name:           "3 routes & 3 routes for max split",
			routes:         threeRoutes,
			maxSplitRoutes: 3,

			expectedRoutesLen: 3,
		},
		{
			name:           "3 routes & 4 routes for max split",
			routes:         threeRoutes,
			maxSplitRoutes: 4,

			expectedRoutesLen: 3,
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {

			routes := usecase.CutRoutesForSplits(tc.maxSplitRoutes, tc.routes)

			s.Require().Len(routes, tc.expectedRoutesLen)
		})
	}
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
