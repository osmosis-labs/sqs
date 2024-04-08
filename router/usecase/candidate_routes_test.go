package usecase_test

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

var (
	pricingConfig = routertesting.DefaultPricingConfig
)

// Validates that the router returns the correct routes for the given token pair.
func (s *RouterTestSuite) TestGetCandidateRoutesBFS_OSMOATOM() {
	config := routertesting.DefaultRouterConfig
	config.MaxPoolsPerRoute = 5
	config.MaxRoutes = 10

	router, _ := s.SetupMainnetRouter(config, pricingConfig)

	candidateRoutes, err := router.GetCandidateRoutes(UOSMO, ATOM)
	s.Require().NoError(err)

	actualRoutes := candidateRoutes.Routes

	s.Require().Equal(config.MaxRoutes, len(actualRoutes))

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
	config := routertesting.DefaultRouterConfig
	config.MaxPoolsPerRoute = 5
	config.MaxRoutes = 10
	config.MinOSMOLiquidity = 1000

	pricingConfig := routertesting.DefaultPricingConfig

	router, _ := s.SetupMainnetRouter(config, pricingConfig)

	candidateRoutesUOSMOIn, err := router.GetCandidateRoutes(UOSMO, stOSMO)
	s.Require().NoError(err)

	actualRoutesUOSMOIn := candidateRoutesUOSMOIn.Routes

	// Invert
	candidateRoutesstOSMOIn, err := router.GetCandidateRoutes(stOSMO, UOSMO)
	s.Require().NoError(err)

	actualRoutesStOSMOIn := candidateRoutesstOSMOIn.Routes

	s.Require().NotZero(len(actualRoutesUOSMOIn))
	s.Require().Equal(len(actualRoutesUOSMOIn), len(actualRoutesStOSMOIn))
}

func (s *RouterTestSuite) TestGetCandidateRoutesBFS_ATOMUSDT() {
	config := domain.RouterConfig{
		PreferredPoolIDs:          []uint64{},
		MaxPoolsPerRoute:          4,
		MaxRoutes:                 5,
		MaxSplitRoutes:            3,
		MaxSplitIterations:        10,
		MinOSMOLiquidity:          10000, // 10_000 OSMO
		RouteUpdateHeightInterval: 0,
		RouteCacheEnabled:         false,
	}

	router, _ := s.SetupMainnetRouter(config, pricingConfig)

	candidateRoutesUOSMOIn, err := router.GetCandidateRoutes(ATOM, USDT)
	s.Require().NoError(err)

	s.Require().Greater(len(candidateRoutesUOSMOIn.Routes), 0)
}

// Validate that can find at least 1 route with no error for top 10
// pairs by volume.
func (s *RouterTestSuite) TestGetCandidateRoutesBFS_Top10VolumePairs() {
	config := routertesting.DefaultRouterConfig
	config.MaxPoolsPerRoute = 3
	config.MaxRoutes = 10
	router, _ := s.SetupMainnetRouter(config, pricingConfig)

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

	for i := 0; i < len(top10ByVolumeDenoms); i++ {
		for j := i + 1; j < len(top10ByVolumeDenoms); j++ {
			tokenI := top10ByVolumeDenoms[i]
			tokenJ := top10ByVolumeDenoms[j]

			candidateRoutes, err := router.GetCandidateRoutes(tokenI, tokenJ)
			s.Require().NoError(err)
			s.Require().Greater(len(candidateRoutes.Routes), 0, "tokenI: %s, tokenJ: %s", tokenI, tokenJ)

			candidateRoutes, err = router.GetCandidateRoutes(tokenJ, tokenI)
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
