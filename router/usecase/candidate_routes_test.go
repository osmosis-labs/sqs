package usecase_test

import (
	"slices"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain"
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
func (s *RouterTestSuite) TestCandidateRouteSearcher_HappyPath() {

	mainnetState := s.SetupMainnetState()

	usecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	tests := []struct {
		name          string
		tokenIn       sdk.Coin
		tokenOutDenom string
	}{
		{
			name:          "UOSMO -> USDT",
			tokenIn:       sdk.NewCoin(UOSMO, one),
			tokenOutDenom: USDT,
		},
		{
			name:          "UMEE -> AKT",
			tokenIn:       sdk.NewCoin(UMEE, one),
			tokenOutDenom: AKT,
		},
		{
			name:          "ALLBTC -> USDC",
			tokenIn:       sdk.NewCoin(ALLBTC, one),
			tokenOutDenom: USDC,
		},
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

func (s *RouterTestSuite) validateExpectedPoolIDOneHopRoute(route sqsdomain.CandidateRoute, expectedPoolID uint64) {
	routePools := route.Pools
	s.Require().Equal(1, len(routePools))
	s.Require().Equal(expectedPoolID, routePools[0].ID)
}
