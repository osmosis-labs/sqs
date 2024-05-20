package usecase_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
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
		maxPoolsPerRoute    = 5
		maxRoutes           = 10
		minPoolLiquidityCap = 1000
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
		maxRoutes           = 5
		maxPoolsPerRoute    = 4
		minPoolLiquidityCap = 10000
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
