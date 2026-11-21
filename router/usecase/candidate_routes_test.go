package usecase_test

import (
	"context"
	"slices"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	sqsatomic "github.com/osmosis-labs/sqs/domain/sync/atomic"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/repository"
	"github.com/osmosis-labs/sqs/router/usecase"
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
			candidateRoutes, err := usecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(context.Background(), tc.tokenIn, tc.tokenOutDenom, candidateRouteOptions)
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
			candidateRoutes, err := usecase.CandidateRouteSearcher.FindCandidateRoutesInGivenOut(context.Background(), tc.tokenOut, tc.tokenInDenom, candidateRouteOptions)
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
	candidateRoutes, err := usecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(context.Background(), oneOSMOIn, ATOM, candidateRouteOptions)
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
	candidateRoutes, err = usecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(context.Background(), oneOSMOIn, ATOM, candidateRouteOptions)
	s.Require().NoError(err)

	didFindExpectedPoolID = foundExpectedPoolID(expectedPoolID, candidateRoutes.Routes)
	s.Require().False(didFindExpectedPoolID)
}

// This test validates that the skip pool candidate route option works as intended
// by setting up a test between OSMO and ATOM and excluding pool ID 1 via an option filter.
func (s *RouterTestSuite) TestCandidateRouteSearcherInGivenOut_SkipPoolOption() {
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
	candidateRoutes, err := usecase.CandidateRouteSearcher.FindCandidateRoutesInGivenOut(context.Background(), oneOSMOIn, ATOM, candidateRouteOptions)
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
	candidateRoutes, err = usecase.CandidateRouteSearcher.FindCandidateRoutesInGivenOut(context.Background(), oneOSMOIn, ATOM, candidateRouteOptions)
	s.Require().NoError(err)

	didFindExpectedPoolID = foundExpectedPoolID(expectedPoolID, candidateRoutes.Routes)
	s.Require().False(didFindExpectedPoolID)
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

// This test validates that pools listed in the router config's excluded pool IDs
// are never returned by the candidate route search, without any per-call pool filter option.
// In-given-out search reuses the out-given-in search, so one direction covers both.
func (s *RouterTestSuite) TestCandidateRouteSearcher_ExcludedPoolIDs() {
	mainnetState := s.SetupMainnetState()

	// OSMO/ATOM pool
	const excludedPoolID = uint64(1)

	routerConfig := routertesting.DefaultRouterConfig
	routerConfig.ExcludedPoolIDs = []uint64{excludedPoolID}

	defaultUsecase := s.SetupRouterAndPoolsUsecase(mainnetState)
	excludingUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(routerConfig))

	candidateRouteOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           routerConfig.MaxRoutes,
		MaxPoolsPerRoute:    routerConfig.MaxPoolsPerRoute,
		MinPoolLiquidityCap: routerConfig.MinPoolLiquidityCap,
	}

	oneOSMO := sdk.NewCoin(UOSMO, defaultAmount)

	// Without the exclusion, the pool is a candidate.
	candidateRoutes, err := defaultUsecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(context.Background(), oneOSMO, ATOM, candidateRouteOptions)
	s.Require().NoError(err)
	s.Require().True(foundExpectedPoolID(excludedPoolID, candidateRoutes.Routes))

	// System under test.
	candidateRoutes, err = excludingUsecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(context.Background(), oneOSMO, ATOM, candidateRouteOptions)
	s.Require().NoError(err)
	s.Require().NotEmpty(candidateRoutes.Routes)
	s.Require().False(foundExpectedPoolID(excludedPoolID, candidateRoutes.Routes))
}

// Validates that a search for a token that no pool holds, which would otherwise queue every path up to max
// pools per route, stops queuing at the bound, and that searches for routable tokens are unchanged by the default bound.
func (s *RouterTestSuite) TestCandidateRouteSearcher_QueuedPathBound() {
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	candidateRouteOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           20,
		MaxPoolsPerRoute:    4,
		MinPoolLiquidityCap: 0,
		PoolFiltersAnyOf:    []domain.CandidateRoutePoolFiltrerCb{domain.ShouldSkipOrderbookPool},
	}
	tenUSDC := sdk.NewCoin(USDC, osmomath.NewInt(10_000_000))

	// An unroutable token reaches the bound and finds no routes.
	const bound = 1_000
	routes, queuedPaths, capped, err := usecase.FindCandidateRoutesOutGivenInWithBound(context.Background(), mainnetUsecase.CandidateRouteSearcher, tenUSDC, "unroutable", candidateRouteOptions, bound)
	s.Require().NoError(err)
	s.Require().Empty(routes)
	s.Require().Equal(bound, queuedPaths)
	s.Require().True(capped)

	// A search whose request is gone stops with the context's error.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err = usecase.FindCandidateRoutesOutGivenInWithBound(cancelledCtx, mainnetUsecase.CandidateRouteSearcher, tenUSDC, "unroutable", candidateRouteOptions, bound)
	s.Require().ErrorIs(err, context.Canceled)

	// Routable tokens find the same routes with the default bound as without one.
	previousBound := usecase.SetMaxCandidateRouteSearchPaths(1 << 30)
	unbounded := map[string]ingesttypes.CandidateRoutes{}
	for _, denom := range routertesting.MainnetDenoms {
		candidateRoutes, err := mainnetUsecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(context.Background(), tenUSDC, denom, candidateRouteOptions)
		s.Require().NoError(err)
		unbounded[denom] = candidateRoutes
	}
	usecase.SetMaxCandidateRouteSearchPaths(previousBound)

	for _, denom := range routertesting.MainnetDenoms {
		candidateRoutes, err := mainnetUsecase.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(context.Background(), tenUSDC, denom, candidateRouteOptions)
		s.Require().NoError(err)
		s.Require().Equal(unbounded[denom], candidateRoutes, denom)
	}
}

// TestCandidateRouteSearcherInGivenOut_FirstPoolCapacity checks the first pool balance check of the exact-out search:
// a pool holding a split share of the token out is kept, an alloy transmuter is checked for the token out balance
// unless the token out is its alloyed LP share, and the pools of a route stay ordered from the token out side.
func (s *RouterTestSuite) TestCandidateRouteSearcherInGivenOut_FirstPoolCapacity() {
	const out, mid, in = "uout", "umid", "uinn"
	pool := func(id uint64, denoms []string, balances sdk.Coins, alloyedDenom string) domain.CandidatePoolWrapper {
		return domain.CandidatePoolWrapper{
			ID: id, PoolDenoms: denoms, Balances: balances, PoolLiquidityCap: sqsatomic.NewUint64(1_000_000),
			IsAlloyTransmuter: alloyedDenom != "", AlloyedDenom: alloyedDenom,
		}
	}
	tokenOut := sdk.NewCoin(out, osmomath.NewInt(100))

	find := func(pools map[string][]domain.CandidatePoolWrapper, maxSplitRoutes int) [][]uint64 {
		holder := routerrepo.New(noOpLogger)
		data := map[string]*domain.CandidateRouteDenomData{}
		for denom, p := range pools {
			data[denom] = &domain.CandidateRouteDenomData{SortedPools: p}
		}
		holder.SetCandidateRouteSearchData(data)
		options := domain.CandidateRouteSearchOptions{MaxRoutes: 5, MaxPoolsPerRoute: 3, MaxSplitRoutes: maxSplitRoutes}
		routes, err := usecase.NewCandidateRouteFinder(holder, nil, noOpLogger).FindCandidateRoutesInGivenOut(context.Background(), tokenOut, in, options)
		if err != nil {
			return nil
		}
		ids := [][]uint64{}
		for _, r := range routes.Routes {
			poolIDs := []uint64{}
			for _, p := range r.Pools {
				poolIDs = append(poolIDs, p.ID)
			}
			ids = append(ids, poolIDs)
		}
		return ids
	}

	// Pool 1 holds 40 of the 100 out, pool 2 links mid to in.
	partial := pool(1, []string{out, mid}, sdk.NewCoins(sdk.NewInt64Coin(out, 40), sdk.NewInt64Coin(mid, 1000)), "")
	link := pool(2, []string{mid, in}, sdk.NewCoins(sdk.NewInt64Coin(mid, 1000), sdk.NewInt64Coin(in, 1000)), "")
	pools := map[string][]domain.CandidatePoolWrapper{out: {partial}, mid: {partial, link}, in: {link}}

	s.Require().Empty(find(pools, 0), "a pool short of the whole amount is skipped when the search has no split share")
	s.Require().Equal([][]uint64{{1, 2}}, find(pools, 3), "kept with its pool order from the token out side")

	// An alloy transmuter holding 10 of the out variant is skipped, but kept when out is its alloyed LP share.
	alloy := func(alloyedDenom string) map[string][]domain.CandidatePoolWrapper {
		t := pool(3, []string{out, in}, sdk.NewCoins(sdk.NewInt64Coin(in, 1000), sdk.NewInt64Coin(out, 10)), alloyedDenom)
		return map[string][]domain.CandidatePoolWrapper{out: {t}, in: {t}}
	}
	s.Require().Empty(find(alloy("other"), 3))
	s.Require().Equal([][]uint64{{3}}, find(alloy(out), 3))
}
