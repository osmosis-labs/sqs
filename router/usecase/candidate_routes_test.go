package usecase_test

import (
	"slices"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

var (
	pricingConfig = routertesting.DefaultPricingConfig
	noOpLogger    = &log.NoOpLogger{}

	one = osmomath.OneInt()
)

// Validates that the router returns the correct routes for the given token pair.
func (s *RouterTestSuite) TestGetCandidateRoutesBFS_OSMOATOM() {
	var (
		maxPoolsPerRoute = 5
		maxRoutes        = 10
	)

	mainnetState := s.SetupMainnetState()

	// Prepare valid and sorted pools
	poolsAboveMinLiquidity := routertesting.PrepareValidSortedRouterPools(mainnetState.Pools, defaultRouterConfig.MinPoolLiquidityCap)

	// System under test.
	candidateRoutes, err := routerusecase.GetCandidateRoutes(poolsAboveMinLiquidity, sdk.NewCoin(UOSMO, one), ATOM, maxRoutes, maxPoolsPerRoute, noOpLogger)
	s.Require().NoError(err)

	actualRoutes := candidateRoutes.Routes

	s.Require().Equal(maxRoutes, len(actualRoutes))

	// https://app.osmosis.zone/pool/1135
	s.validateExpectedPoolIDOneHopRoute(actualRoutes[0], 1135)

	// TODO need to improve comparison between CL and CFMM pools
	// There is actually pool 1 with much higher liquidity here but it is not returned because it is a CFMM pool.
	// https://app.osmosis.zone/pool/1265
	s.validateExpectedPoolIDOneHopRoute(actualRoutes[1], 1265)
}

// Validates that the router returns the correct routes for the given token pair.
// Inverting the swap direction should return the same routes.
func (s *RouterTestSuite) TestGetCandidateRoutesBFS_OSMOstOSMO() {
	var (
		maxPoolsPerRoute           = 5
		maxRoutes                  = 10
		minPoolLiquidityCap uint64 = 1000
	)

	mainnetState := s.SetupMainnetState()

	// Prepare valid and sorted pools
	poolsAboveMinLiquidity := routertesting.PrepareValidSortedRouterPools(mainnetState.Pools, minPoolLiquidityCap)

	candidateRoutesUOSMOIn, err := routerusecase.GetCandidateRoutes(poolsAboveMinLiquidity, sdk.NewCoin(UOSMO, one), stOSMO, maxRoutes, maxPoolsPerRoute, noOpLogger)
	s.Require().NoError(err)

	actualRoutesUOSMOIn := candidateRoutesUOSMOIn.Routes

	// Invert
	candidateRoutesstOSMOIn, err := routerusecase.GetCandidateRoutes(poolsAboveMinLiquidity, sdk.NewCoin(stOSMO, one), UOSMO, maxRoutes, maxPoolsPerRoute, noOpLogger)
	s.Require().NoError(err)

	actualRoutesStOSMOIn := candidateRoutesstOSMOIn.Routes

	s.Require().NotZero(len(actualRoutesUOSMOIn))
	s.Require().Equal(len(actualRoutesUOSMOIn), len(actualRoutesStOSMOIn))
}

func (s *RouterTestSuite) TestGetCandidateRoutesBFS_ATOMUSDT() {
	var (
		maxRoutes                  = 5
		maxPoolsPerRoute           = 4
		minPoolLiquidityCap uint64 = 10000
	)

	mainnetState := s.SetupMainnetState()

	// Prepare valid and sorted pools
	poolsAboveMinLiquidity := routertesting.PrepareValidSortedRouterPools(mainnetState.Pools, minPoolLiquidityCap)

	candidateRoutesUOSMOIn, err := routerusecase.GetCandidateRoutes(poolsAboveMinLiquidity, sdk.NewCoin(ATOM, one), USDT, maxRoutes, maxPoolsPerRoute, noOpLogger)
	s.Require().NoError(err)

	s.Require().Greater(len(candidateRoutesUOSMOIn.Routes), 0)
}

func (s *RouterTestSuite) TestGetCandidateRoutes_USDT_USDC() {

	mainnetState := s.SetupMainnetState()

	// Prepare valid and sorted pools
	poolsAboveMinLiquidity := routertesting.PrepareValidSortedRouterPools(mainnetState.Pools, routertesting.DefaultPricingRouterConfig.MinPoolLiquidityCap)

	candidateRoutesUOSMOIn, err := routerusecase.GetCandidateRoutes(poolsAboveMinLiquidity, sdk.NewCoin(USDT, one), USDC, routertesting.DefaultPricingRouterConfig.MaxRoutes, routertesting.DefaultPricingRouterConfig.MaxPoolsPerRoute, noOpLogger)
	s.Require().NoError(err)

	s.Require().Greater(len(candidateRoutesUOSMOIn.Routes), 0)
}

// TestCandidateRouteSearcher is a happy path test case of the candidate route search algorithm.
// For every token pair, it finds the candidate routes and validates that the routes are within the configured bounds.
// That is, that the number of routes are non-zero, less than or equal to the max number of routes.
// Additionally, for every route, the test validates that the denoms are indeed present in the pool denoms of each pool.
// Lastly, that the number of pools in route is less than or equal to the max number of pools per route and greater than zero
// while also above the minimum pool liquidity cap.
func (s *RouterTestSuite) TestCandidateRouteSearcher_HappyPath() {

	mainnetState := s.SetupMainnetState()

	usecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	tests := []struct {
		name          string
		tokenIn       sdk.Coin
		tokenOutDenom string
	}{
		// {
		// 	name:          "UOSMO -> USDT",
		// 	tokenIn:       sdk.NewCoin(UOSMO, one),
		// 	tokenOutDenom: USDT,
		// },
		// {
		// 	name:          "UMEE -> AKT",
		// 	tokenIn:       sdk.NewCoin(UMEE, one),
		// 	tokenOutDenom: AKT,
		// },
		// {
		// 	name:          "ALLBTC -> USDC",
		// 	tokenIn:       sdk.NewCoin(ALLBTC, one),
		// 	tokenOutDenom: USDC,
		// },
		{
			name:          "ALLBTC -> TIA",
			tokenIn:       sdk.NewCoin(ALLBTC, one),
			tokenOutDenom: TIA,
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
			candidateRoutes, err := usecase.CandidateRouteSearcher.FindCandidateRoutes(tc.tokenIn, tc.tokenOutDenom, candidateRouteOptions)
			s.Require().NoError(err)

			// Validate that at least one route found
			s.Require().Greater(len(candidateRoutes.Routes), 0)
			// Validate that the number of routes found is less than or equal to the max number of routes.
			s.Require().LessOrEqual(len(candidateRoutes.Routes), candidateRouteOptions.MaxRoutes)
			// Validate that the unieque pools are non-empty.
			s.Require().Greater(len(candidateRoutes.UniquePoolIDs), 0)

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

					s.Require().True(poolInRoute.GetPoolLiquidityCap().GTE(expectedMinPoolLiquidityCapInt), "poolID: %d, expectedMinPoolLiquidityCapInt: %s, poolInRoute.GetPoolLiquidityCap(): %s", pool.ID, expectedMinPoolLiquidityCapInt, poolInRoute.GetPoolLiquidityCap())

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

// Validate that can find at least 1 route with no error for top 10
// pairs by volume.
func (s *RouterTestSuite) TestGetCandidateRoutesBFS_Top10VolumePairs() {
	var (
		maxRoutes        = 10
		maxPoolsPerRoute = 3
	)

	mainnetState := s.SetupMainnetState()

	// Manually taken from https://info.osmosis.zone/ in Nov 2023.
	top10ByVolumeDenoms := []string{
		UOSMO,
		ATOM,
		stOSMO,
		stATOM,
		USDC,
		USDCaxl,
		USDT,
		WBTC,
		ETH,
		AKT,
	}

	one := osmomath.OneInt()

	// Prepare valid and sorted pools
	poolsAboveMinLiquidity := routertesting.PrepareValidSortedRouterPools(mainnetState.Pools, defaultRouterConfig.MinPoolLiquidityCap)

	for i := 0; i < len(top10ByVolumeDenoms); i++ {
		for j := i + 1; j < len(top10ByVolumeDenoms); j++ {
			tokenI := top10ByVolumeDenoms[i]
			tokenJ := top10ByVolumeDenoms[j]

			candidateRoutes, err := routerusecase.GetCandidateRoutes(poolsAboveMinLiquidity, sdk.NewCoin(tokenI, one), tokenJ, maxRoutes, maxPoolsPerRoute, noOpLogger)
			s.Require().NoError(err)
			s.Require().Greater(len(candidateRoutes.Routes), 0, "tokenI: %s, tokenJ: %s", tokenI, tokenJ)

			candidateRoutes, err = routerusecase.GetCandidateRoutes(poolsAboveMinLiquidity, sdk.NewCoin(tokenJ, one), tokenI, maxRoutes, maxPoolsPerRoute, noOpLogger)
			s.Require().NoError(err)
			s.Require().Greater(len(candidateRoutes.Routes), 0, "tokenJ: %s, tokenI: %s", tokenJ, tokenI)
		}
	}
}

func (s *RouterTestSuite) validateExpectedPoolIDOneHopRoute(route sqsdomain.CandidateRoute, expectedPoolID uint64) {
	routePools := route.Pools
	s.Require().Equal(1, len(routePools))
	s.Require().Equal(expectedPoolID, routePools[0].ID)
}
