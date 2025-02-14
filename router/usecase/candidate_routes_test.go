package usecase_test

import (
	"slices"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
)

var (
	pricingConfig = routertesting.DefaultPricingConfig
	noOpLogger    = &log.NoOpLogger{}

	one = osmomath.OneInt()
)

// TestCandidateRouteSearcher is a happy path test case of the candidate route search algorithm.
// For every token pair, it finds the candidate routes and validates that the routes are within the configured bounds.
// That is, that the number of routes are non-zero, less than or equal to the max number of routes.
// Additionally, for every route, the test validates that the denoms are indeed present in the pool denoms of each pool.
// Lastly, that the number of pools in route is less than or equal to the max number of pools per route and greater than zero
// while also above the minimum pool liquidity cap.
func (s *RouterTestSuite) TestCandidateRouteSearcherOutGivenIn_HappyPath() {

	mainnetState := s.SetupMainnetState()

	usecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	tests := []struct {
		name          string
		tokenIn       sdk.Coin
		tokenOutDenom string
		uniquePoolIDs map[uint64]struct{}
		routes        int
	}{
		{
			name:          "UOSMO -> USDT",
			tokenIn:       sdk.NewCoin(UOSMO, one),
			tokenOutDenom: USDT,
			routes:        20,
			uniquePoolIDs: map[uint64]struct{}{
				1066: {},
				1077: {},
				1078: {},
				1079: {},
				1081: {},
				1094: {},
				1110: {},
				1133: {},
				1134: {},
				1135: {},
				1159: {},
				1189: {},
				1205: {},
				1220: {},
				1221: {},
				1261: {},
				1263: {},
				1265: {},
				1279: {},
				1281: {},
				1399: {},
				1400: {},
				1464: {},
				1477: {},
				1895: {},
			},
		},
		{
			name:          "UMEE -> AKT",
			tokenIn:       sdk.NewCoin(UMEE, one),
			tokenOutDenom: AKT,
			routes:        20,
			uniquePoolIDs: map[uint64]struct{}{
				1077: {},
				1078: {},
				1079: {},
				1093: {},
				1104: {},
				1110: {},
				1135: {},
				1205: {},
				1220: {},
				1221: {},
				1263: {},
				1265: {},
				1301: {},
				1368: {},
				1400: {},
				1464: {},
				1480: {},
				3:    {},
				4:    {},
				641:  {},
				643:  {},
			},
		},
		{
			name:          "ALLBTC -> USDC",
			tokenIn:       sdk.NewCoin(ALLBTC, one),
			tokenOutDenom: USDC,
			routes:        20,
			uniquePoolIDs: map[uint64]struct{}{
				1221: {},
				1253: {},
				1263: {},
				1264: {},
				1268: {},
				1277: {},
				1278: {},
				1422: {},
				1433: {},
				1434: {},
				1435: {},
				1436: {},
				1437: {},
				1440: {},
				1441: {},
				1464: {},
				1490: {},
				1705: {},
				1868: {},
				1904: {},
				1930: {},
				1942: {},
			},
		},
		{
			name:          "ALLBTC -> TIA",
			tokenIn:       sdk.NewCoin(ALLBTC, one),
			tokenOutDenom: TIA,
			routes:        20,
			uniquePoolIDs: map[uint64]struct{}{
				1247: {},
				1248: {},
				1249: {},
				1321: {},
				1347: {},
				1433: {},
				1434: {},
				1436: {},
				1437: {},
				1478: {},
				1579: {},
				1868: {},
				1904: {},
				1942: {},
			},
		},
	}

	for _, tc := range tests {
		s.T().Run(tc.name, func(t *testing.T) {

			routerConfig := usecase.Router.GetConfig()
			candidateRouteOptions := domain.CandidateRouteSearchOptions{
				MaxRoutes:           routerConfig.MaxRoutes,
				MaxPoolsPerRoute:    routerConfig.MaxPoolsPerRoute,
				MinPoolLiquidityCap: routerConfig.MinPoolLiquidityCap,
			}

			expectedMinPoolLiquidityCapInt := osmomath.NewInt(int64(routerConfig.MinPoolLiquidityCap))

			// System under test
			candidateRoutes, err := usecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(tc.tokenIn, tc.tokenOutDenom, candidateRouteOptions)
			s.Require().NoError(err)

			// Validate that number of routes found is equal to the expected number of routes.
			s.Require().Equal(len(candidateRoutes.Routes), tc.routes)
			// Validate that the number of routes found is less than or equal to the max number of routes.
			s.Require().LessOrEqual(len(candidateRoutes.Routes), candidateRouteOptions.MaxRoutes)
			// Validate that the unieque pools is equal to the expected unique pool IDs.
			s.Require().Equal(candidateRoutes.UniquePoolIDs, tc.uniquePoolIDs)

			// Validate each route and its pools to be within he configured bounds.
			for _, route := range candidateRoutes.Routes {
				// Validate that the route is non-empty.
				s.Require().Greater(len(route.Pools), 0)
				// Validate that the route is less than or equal to the max number of pools per route.
				s.Require().LessOrEqual(len(route.Pools), candidateRouteOptions.MaxPoolsPerRoute)

				curTokenInDenom := tc.tokenIn.Denom

				for _, pool := range route.Pools {
					// Validate that the pool ID is in the unique pool IDs.
					s.Require().Contains(candidateRoutes.UniquePoolIDs, pool.ID)

					// Validate that the pool ID is in the pools above min liquidity.
					poolInRoute, err := usecase.Pools.GetPool(pool.ID)
					s.Require().NoError(err)

					cosmwasmModel := poolInRoute.GetSQSPoolModel().CosmWasmPoolModel
					isOrderbook := cosmwasmModel != nil && cosmwasmModel.IsOrderbook()
					// Note: canonical order books are injected into routes, completely ignoring liquidity caps
					// so we don't need to check for liquidity caps for canonical order books
					if !isOrderbook {
						s.Require().True(poolInRoute.GetPoolLiquidityCap().GTE(expectedMinPoolLiquidityCapInt), "poolID: %d, expectedMinPoolLiquidityCapInt: %s, poolInRoute.GetPoolLiquidityCap(): %s", pool.ID, expectedMinPoolLiquidityCapInt, poolInRoute.GetPoolLiquidityCap())
					}

					// Pool contains token in
					poolDenoms := poolInRoute.GetPoolDenoms()
					s.Require().True(slices.Contains(poolDenoms, curTokenInDenom))

					// Pool contains token out
					tokenOut := pool.TokenOutDenom
					s.Require().True(slices.Contains(poolInRoute.GetPoolDenoms(), tokenOut))

					// Change tokenInDenom to tokenOutDenom for the next iteration
					curTokenInDenom = tokenOut
				}

				// Validate that the resulting token out denom equals to the one set by the test
				// Note that we set he curTokenInDenom to the tokenOutDenom of the last pool in the route
				s.Require().Equal(tc.tokenOutDenom, curTokenInDenom)
			}
		})
	}
}

// TestCandidateRouteSearcher is a happy path test case of the candidate route search algorithm.
// For every token pair, it finds the candidate routes and validates that the routes are within the configured bounds.
// That is, that the number of routes are non-zero, less than or equal to the max number of routes.
// Additionally, for every route, the test validates that the denoms are indeed present in the pool denoms of each pool.
// Lastly, that the number of pools in route is less than or equal to the max number of pools per route and greater than zero
// while also above the minimum pool liquidity cap.
func (s *RouterTestSuite) TestCandidateRouteSearcherInGivenOut_HappyPath() {

	mainnetState := s.SetupMainnetState()

	usecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	tests := []struct {
		name          string
		tokenOut      sdk.Coin
		tokenInDenom  string
		uniquePoolIDs map[uint64]struct{}
		routes        int
	}{
		{
			name:         "UOSMO -> USDT",
			tokenOut:     sdk.NewCoin(UOSMO, one),
			tokenInDenom: USDT,
			routes:       20,
			uniquePoolIDs: map[uint64]struct{}{
				1066: {},
				1077: {},
				1078: {},
				1079: {},
				1081: {},
				1094: {},
				1110: {},
				1133: {},
				1134: {},
				1135: {},
				1159: {},
				1189: {},
				1205: {},
				1220: {},
				1221: {},
				1261: {},
				1263: {},
				1265: {},
				1279: {},
				1281: {},
				1399: {},
				1400: {},
				1464: {},
				1477: {},
				1895: {},
			},
		},
		{
			name:         "UMEE -> AKT",
			tokenOut:     sdk.NewCoin(UMEE, one),
			tokenInDenom: AKT,
			routes:       20,
			uniquePoolIDs: map[uint64]struct{}{
				1077: {},
				1078: {},
				1079: {},
				1093: {},
				1104: {},
				1110: {},
				1135: {},
				1205: {},
				1220: {},
				1221: {},
				1263: {},
				1265: {},
				1301: {},
				1368: {},
				1400: {},
				1464: {},
				1480: {},
				3:    {},
				4:    {},
				641:  {},
				643:  {},
			},
		},
		{
			name:         "ALLBTC -> USDC",
			tokenOut:     sdk.NewCoin(ALLBTC, one),
			tokenInDenom: USDC,
			routes:       20,
			uniquePoolIDs: map[uint64]struct{}{
				1221: {},
				1253: {},
				1263: {},
				1264: {},
				1268: {},
				1277: {},
				1278: {},
				1422: {},
				1433: {},
				1434: {},
				1435: {},
				1436: {},
				1437: {},
				1440: {},
				1441: {},
				1464: {},
				1490: {},
				1705: {},
				1868: {},
				1904: {},
				1930: {},
				1942: {},
			},
		},
		{
			name:         "ALLBTC -> TIA",
			tokenOut:     sdk.NewCoin(ALLBTC, one),
			tokenInDenom: TIA,
			routes:       20,
			uniquePoolIDs: map[uint64]struct{}{
				1247: {},
				1248: {},
				1249: {},
				1321: {},
				1347: {},
				1433: {},
				1434: {},
				1436: {},
				1437: {},
				1478: {},
				1579: {},
				1868: {},
				1904: {},
				1942: {},
			},
		},
	}

	for _, tc := range tests {
		s.T().Run(tc.name, func(t *testing.T) {

			routerConfig := usecase.Router.GetConfig()
			candidateRouteOptions := domain.CandidateRouteSearchOptions{
				MaxRoutes:           routerConfig.MaxRoutes,
				MaxPoolsPerRoute:    routerConfig.MaxPoolsPerRoute,
				MinPoolLiquidityCap: routerConfig.MinPoolLiquidityCap,
			}

			expectedMinPoolLiquidityCapInt := osmomath.NewInt(int64(routerConfig.MinPoolLiquidityCap))

			// System under test
			candidateRoutes, err := usecase.CandidateRouteSearcher.FindCandidateRoutesInGivenOut(tc.tokenOut, tc.tokenInDenom, candidateRouteOptions)
			s.Require().NoError(err)

			// Validate that number of routes found is equal to the expected number of routes.
			s.Require().Equal(len(candidateRoutes.Routes), tc.routes)
			// Validate that the number of routes found is less than or equal to the max number of routes.
			s.Require().LessOrEqual(len(candidateRoutes.Routes), candidateRouteOptions.MaxRoutes)
			// Validate that the unieque pools is equal to the expected unique pool IDs.
			s.Require().Equal(candidateRoutes.UniquePoolIDs, tc.uniquePoolIDs)

			// Validate each route and its pools to be within he configured bounds.
			for _, route := range candidateRoutes.Routes {
				// Validate that the route is non-empty.
				s.Require().Greater(len(route.Pools), 0)
				// Validate that the route is less than or equal to the max number of pools per route.
				s.Require().LessOrEqual(len(route.Pools), candidateRouteOptions.MaxPoolsPerRoute)

				curTokenOutDenom := tc.tokenOut.Denom

				for _, pool := range route.Pools {
					// Validate that the pool ID is in the unique pool IDs.
					s.Require().Contains(candidateRoutes.UniquePoolIDs, pool.ID)

					// Validate that the pool ID is in the pools above min liquidity.
					poolInRoute, err := usecase.Pools.GetPool(pool.ID)
					s.Require().NoError(err)

					cosmwasmModel := poolInRoute.GetSQSPoolModel().CosmWasmPoolModel
					isOrderbook := cosmwasmModel != nil && cosmwasmModel.IsOrderbook()
					// Note: canonical order books are injected into routes, completely ignoring liquidity caps
					// so we don't need to check for liquidity caps for canonical order books
					if !isOrderbook {
						s.Require().True(poolInRoute.GetPoolLiquidityCap().GTE(expectedMinPoolLiquidityCapInt), "poolID: %d, expectedMinPoolLiquidityCapInt: %s, poolInRoute.GetPoolLiquidityCap(): %s", pool.ID, expectedMinPoolLiquidityCapInt, poolInRoute.GetPoolLiquidityCap())
					}

					// Pool contains token out
					poolDenoms := poolInRoute.GetPoolDenoms()
					s.Require().True(slices.Contains(poolDenoms, curTokenOutDenom))

					// Pool contains token in
					tokenIn := pool.TokenInDenom
					s.Require().True(slices.Contains(poolInRoute.GetPoolDenoms(), tokenIn))

					// Change tokenOutDenom to tokenInDenom for the next iteration
					curTokenOutDenom = tokenIn
				}

				// Validate that the resulting token out denom equals to the one set by the test
				// Note that we set he curTokenOutDenom to the tokenInDenom of the last pool in the route
				s.Require().Equal(tc.tokenInDenom, curTokenOutDenom)
			}
		})
	}
}

// This test validates that the skip pool candidate route option works as intended
// by setting up a test between OSMO and ATOM and excluding pool ID 1 via an option filter.
func (s *RouterTestSuite) TestCandidateRouteSearcherOutGivenIn_SkipPoolOption() {
	mainnetState := s.SetupMainnetState()

	usecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	oneOSMOIn := sdk.NewCoin(UOSMO, defaultAmount)

	routerConfig := usecase.Router.GetConfig()
	candidateRouteOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           routerConfig.MaxRoutes,
		MaxPoolsPerRoute:    routerConfig.MaxPoolsPerRoute,
		MinPoolLiquidityCap: routerConfig.MinPoolLiquidityCap,
	}

	// OSMO/ATOM pool
	const expectedPoolID = uint64(1)

	// System under test #1
	candidateRoutes, err := usecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(oneOSMOIn, ATOM, candidateRouteOptions)
	s.Require().NoError(err)

	// Contains default pool ID
	didFindExpectedPoolID := foundExpectedPoolID(expectedPoolID, candidateRoutes.Routes)

	s.Require().True(didFindExpectedPoolID)

	// Now, add a filter
	candidateRoutePoolIDFilter := domain.CandidateRoutePoolIDFilterOptionCb{
		PoolIDsToSkip: map[uint64]struct{}{
			expectedPoolID: {},
		},
	}

	candidateRouteOptions.PoolFiltersAnyOf = []domain.CandidateRoutePoolFiltrerCb{
		candidateRoutePoolIDFilter.ShouldSkipPool,
	}

	// System under test #2
	candidateRoutes, err = usecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(oneOSMOIn, ATOM, candidateRouteOptions)
	s.Require().NoError(err)

	didFindExpectedPoolID = foundExpectedPoolID(expectedPoolID, candidateRoutes.Routes)
	s.Require().False(didFindExpectedPoolID)
}

func (s *RouterTestSuite) validateExpectedPoolIDOneHopRoute(route ingesttypes.CandidateRoute, expectedPoolID uint64) {
	routePools := route.Pools
	s.Require().Equal(1, len(routePools))
	s.Require().Equal(expectedPoolID, routePools[0].ID)
}

// returns true if at least one roue contains a one-hop route with an expected pool ID
func foundExpectedPoolID(expectedPoolID uint64, routes []ingesttypes.CandidateRoute) bool {
	for _, route := range routes {
		routePools := route.Pools

		for _, pool := range routePools {
			if pool.ID == expectedPoolID {
				return true
			}
		}
	}

	return false
}
