package usecase_test

import (
	"context"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/osmoutils/coinutil"
	"github.com/osmosis-labs/osmosis/osmoutils/osmoassert"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	poolsusecase "github.com/osmosis-labs/sqs/pools/usecase"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/osmosis-labs/sqs/router/usecase"
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
)

const (
	defaultPoolID = uint64(1)
)

var (
	UOSMO   = routertesting.UOSMO
	ATOM    = routertesting.ATOM
	stOSMO  = routertesting.STOSMO
	stATOM  = routertesting.STATOM
	USDC    = routertesting.USDC
	USDCaxl = routertesting.USDCaxl
	USDT    = routertesting.USDT
	WBTC    = routertesting.WBTC
	ETH     = routertesting.ETH
	AKT     = routertesting.AKT
	UMEE    = routertesting.UMEE
	UION    = routertesting.UION
)

// TODO: copy exists in candidate_routes_test.go - share & reuse
var (
	DefaultTakerFee     = osmomath.MustNewDecFromStr("0.002")
	DefaultPoolBalances = sdk.NewCoins(
		sdk.NewCoin(DenomOne, DefaultAmt0),
		sdk.NewCoin(DenomTwo, DefaultAmt1),
	)
	DefaultSpreadFactor = osmomath.MustNewDecFromStr("0.005")

	DefaultMockPool = &mocks.MockRoutablePool{
		ID:                   defaultPoolID,
		Denoms:               []string{DenomOne, DenomTwo},
		TotalValueLockedUSDC: osmomath.NewInt(10),
		PoolType:             poolmanagertypes.Balancer,
		Balances:             DefaultPoolBalances,
		TakerFee:             DefaultTakerFee,
		SpreadFactor:         DefaultSpreadFactor,
	}
	EmptyRoute          = route.RouteImpl{}
	EmptyCandidateRoute = sqsdomain.CandidateRoute{}

	// Test denoms
	DenomOne   = routertesting.DenomOne
	DenomTwo   = routertesting.DenomTwo
	DenomThree = routertesting.DenomThree
	DenomFour  = routertesting.DenomFour
	DenomFive  = routertesting.DenomFive
	DenomSix   = routertesting.DenomSix
)

// This test validates that we are able to split over multiple routes.
// We know that higher liquidity pools should be more optimal than the lower liquidity pools
// all else equal.
// Given that, we initialize several pools with different amounts of liquidity.
// We define an expected order of the amountsIn and Out for the split routes based on liquidity.
// Lastly, we assert that the actual order of the split routes is the same as the expected order.
func (s *RouterTestSuite) TestGetBestSplitRoutesQuote() {
	type routeWithOrder struct {
		route domain.SplitRoute
		order int
	}

	s.Setup()

	xLiquidity := sdk.NewCoins(
		sdk.NewCoin(DenomOne, sdk.NewInt(1_000_000_000_000)),
		sdk.NewCoin(DenomTwo, sdk.NewInt(2_000_000_000_000)),
	)

	// X Liquidity
	defaultBalancerPoolID := s.PrepareBalancerPoolWithCoins(xLiquidity...)

	// 2X liquidity
	// Note that the second pool has more liquidity than the first so it should be preferred
	secondBalancerPoolIDSameDenoms := s.PrepareBalancerPoolWithCoins(coinutil.MulRaw(xLiquidity, 2)...)

	// 4X liquidity
	// Note that the third pool has more liquidity than first and second so it should be preferred
	thirdBalancerPoolIDSameDenoms := s.PrepareBalancerPoolWithCoins(coinutil.MulRaw(xLiquidity, 4)...)

	// Get the defaultBalancerPool from the store
	defaultBalancerPool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, defaultBalancerPoolID)
	s.Require().NoError(err)

	// Get the secondBalancerPool from the store
	secondBalancerPoolSameDenoms, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, secondBalancerPoolIDSameDenoms)
	s.Require().NoError(err)

	// Get the thirdBalancerPool from the store
	thirdBalancerPoolSameDenoms, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, thirdBalancerPoolIDSameDenoms)
	s.Require().NoError(err)

	tests := map[string]struct {
		maxSplitIterations int

		routes      []route.RouteImpl
		tokenIn     sdk.Coin
		expectError error

		expectedTokenOutDenom string

		// Ascending order in terms of which route is preferred
		// and uses the largest amount of the token in
		expectedProportionInOrder []int
	}{
		"valid single route": {
			routes: []route.RouteImpl{
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), defaultBalancerPool),
				})},
			tokenIn: sdk.NewCoin(DenomTwo, sdk.NewInt(100)),

			expectedTokenOutDenom: DenomOne,

			expectedProportionInOrder: []int{0},
		},
		"valid two route single hop": {
			routes: []route.RouteImpl{
				// Route 1
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), defaultBalancerPool),
				}),

				// Route 2
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithPoolID(mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), secondBalancerPoolSameDenoms), 2),
				}),
			},

			maxSplitIterations: 10,

			tokenIn: sdk.NewCoin(DenomTwo, sdk.NewInt(5_000_000)),

			expectedTokenOutDenom: DenomOne,

			// Route 2 is preferred because it has 2x the liquidity of Route 1
			expectedProportionInOrder: []int{0, 1},
		},
		"valid three route single hop": {
			routes: []route.RouteImpl{
				// Route 1
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), defaultBalancerPool),
				}),

				// Route 2
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithPoolID(mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), thirdBalancerPoolSameDenoms), 3),
				}),

				// Route 3
				WithRoutePools(route.RouteImpl{}, []sqsdomain.RoutablePool{
					mocks.WithPoolID(mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultMockPool, DenomOne), secondBalancerPoolSameDenoms), 2),
				}),
			},

			maxSplitIterations: 17,

			tokenIn: sdk.NewCoin(DenomTwo, sdk.NewInt(56_789_321)),

			expectedTokenOutDenom: DenomOne,

			// Route 2 is preferred because it has 4x the liquidity of Route 1
			// and 2X the liquidity of Route 3
			expectedProportionInOrder: []int{2, 0, 1},
		},

		// TODO: cover error cases
		// TODO: multi route multi hop
		// TODO: assert that split ratios are correct
	}

	for name, tc := range tests {
		s.Run(name, func() {
			quote, err := routerusecase.GetSplitQuote(context.TODO(), tc.routes, tc.tokenIn)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(tc.expectError, err)
				return
			}
			s.Require().NoError(err)
			s.Require().Equal(tc.tokenIn, quote.GetAmountIn())

			quoteCoinOut := quote.GetAmountOut()
			// We only validate that some amount is returned. The correctness of the amount is to be calculated at a different level
			// of abstraction.
			s.Require().NotNil(quoteCoinOut)

			// Validate that amounts in in the quote split routes add up to the original amount in
			routes := quote.GetRoute()
			actualTotalFromSplits := sdk.ZeroInt()
			for _, splitRoute := range routes {
				actualTotalFromSplits = actualTotalFromSplits.Add(splitRoute.GetAmountIn())
			}

			// Error tolerance of 1 to account for the rounding differences
			errTolerance := osmomath.ErrTolerance{
				AdditiveTolerance: osmomath.OneDec(),
			}
			osmoassert.Equal(s.T(), errTolerance, tc.tokenIn.Amount, actualTotalFromSplits)

			// Route must not be nil
			actualRoutes := quote.GetRoute()
			s.Require().NotNil(actualRoutes)

			s.Require().Equal(len(tc.expectedProportionInOrder), len(actualRoutes))

			routesWithOrder := make([]routeWithOrder, len(actualRoutes))
			for i, route := range actualRoutes {
				routesWithOrder[i] = routeWithOrder{
					route: route,
					order: tc.expectedProportionInOrder[i],
				}
			}

			// Sort actual routes in the expected order
			// and assert that the token in is strictly decreasing
			sort.Slice(routesWithOrder, func(i, j int) bool {
				return routesWithOrder[i].order < routesWithOrder[j].order
			})
			s.Require().NotEmpty(routesWithOrder)

			// Iterate over sorted routes with order and validate that the
			// amounts in and out are strictly decreasing as per the expected order.
			previousRouteAmountIn := routesWithOrder[0].route.GetAmountIn()
			previousRouteAmountOut := routesWithOrder[0].route.GetAmountOut()
			for i := 1; i < len(routesWithOrder)-1; i++ {
				currentRouteAmountIn := routesWithOrder[i].route.GetAmountIn()
				currentRouteAmountOut := routesWithOrder[i].route.GetAmountOut()

				// Both in and out amounts must be strictly decreasing
				s.Require().True(previousRouteAmountIn.GT(currentRouteAmountIn))
				s.Require().True(previousRouteAmountOut.GT(currentRouteAmountOut))
			}
		})
	}
}

// This test ensures strict route validation.
// See individual test cases for details.
func (s *RouterTestSuite) TestValidateAndFilterRoutes() {

	defaultDenomOneTwoOutTwoPool := usecase.CandidatePoolWrapper{
		CandidatePool: sqsdomain.CandidatePool{
			ID:            defaultPoolID,
			TokenOutDenom: DenomTwo,
		},
		PoolDenoms: []string{DenomOne, DenomTwo},
	}

	tests := map[string]struct {
		routes                    [][]usecase.CandidatePoolWrapper
		tokenInDenom              string
		expectError               error
		expectFiltered            bool
		expectFilteredRouteLength int
	}{
		"valid single route single hop": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					defaultDenomOneTwoOutTwoPool,
				},
			},

			tokenInDenom: DenomOne,
		},
		"valid single route multi-hop": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					defaultDenomOneTwoOutTwoPool,
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 1,
							TokenOutDenom: DenomThree,
						},
						PoolDenoms: []string{DenomTwo, DenomThree},
					},
				},
			},

			tokenInDenom: DenomOne,
		},
		"valid multi route": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					defaultDenomOneTwoOutTwoPool,
				},
				{
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 1,
							TokenOutDenom: DenomThree,
						},
						PoolDenoms: []string{DenomOne, DenomThree},
					},
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 2,
							TokenOutDenom: DenomTwo,
						},
						PoolDenoms: []string{DenomTwo, DenomThree},
					},
				},
			},

			tokenInDenom: DenomOne,
		},

		// errors

		"error: no pools in route": {
			routes: [][]usecase.CandidatePoolWrapper{
				{},
			},

			tokenInDenom: DenomTwo,

			expectError: usecase.NoPoolsInRouteError{RouteIndex: 0},
		},
		"error: token out mismatch between multiple routes": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					defaultDenomOneTwoOutTwoPool,
				},
				{
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 1,
							TokenOutDenom: DenomThree,
						},
						PoolDenoms: []string{DenomTwo, DenomThree},
					},
				},
			},

			tokenInDenom: DenomTwo,

			expectError: usecase.TokenOutMismatchBetweenRoutesError{TokenOutDenomRouteA: DenomTwo, TokenOutDenomRouteB: DenomThree},
		},
		"error: token in matches token out": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 1,
							TokenOutDenom: DenomOne,
						},
						PoolDenoms: []string{DenomOne, DenomTwo},
					},
				},
			},

			tokenInDenom: DenomOne,

			expectError: usecase.TokenOutDenomMatchesTokenInDenomError{Denom: DenomOne},
		},
		"error: token in does not match pool denoms": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID,
							TokenOutDenom: DenomOne,
						},
						PoolDenoms: []string{DenomOne, DenomTwo},
					},
				},
			},
			tokenInDenom: DenomThree,

			expectError: usecase.PreviousTokenOutDenomNotInPoolError{RouteIndex: 0, PoolId: DefaultMockPool.GetId(), PreviousTokenOutDenom: DenomThree},
		},
		"error: token out does not match pool denoms": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID,
							TokenOutDenom: DenomThree,
						},
						PoolDenoms: []string{DenomOne, DenomTwo},
					},
				},
			},
			tokenInDenom: DenomOne,

			expectError: usecase.CurrentTokenOutDenomNotInPoolError{RouteIndex: 0, PoolId: DefaultMockPool.GetId(), CurrentTokenOutDenom: DenomThree},
		},

		// Routes filtered
		"filtered: token in is in the route": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID,
							TokenOutDenom: DenomTwo,
						},
						PoolDenoms: []string{DenomOne, DenomTwo},
					},
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 1,
							TokenOutDenom: DenomTwo,
						},
						PoolDenoms: []string{DenomTwo, DenomFour},
					},
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 2,
							TokenOutDenom: DenomFour,
						},
						PoolDenoms: []string{DenomTwo, DenomFour},
					},
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 3,
							TokenOutDenom: DenomThree,
						},
						PoolDenoms: []string{DenomFour, DenomOne},
					},
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 4,
							TokenOutDenom: DenomThree,
						},
						PoolDenoms: []string{DenomOne, DenomThree},
					},
				},
			},
			tokenInDenom: DenomOne,

			expectFiltered: true,
		},
		"filtered: token out is in the route": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID,
							TokenOutDenom: DenomTwo,
						},
						PoolDenoms: []string{DenomOne, DenomTwo},
					},
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 1,
							TokenOutDenom: DenomTwo,
						},
						PoolDenoms: []string{DenomTwo, DenomFour},
					},
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID + 2,
							TokenOutDenom: DenomTwo,
						},
						PoolDenoms: []string{DenomTwo, DenomFour},
					},
				},
			},
			tokenInDenom: DenomOne,

			expectFiltered: true,
		},
		"filtered: same pool id within only route": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID,
							TokenOutDenom: DenomTwo,
						},
						PoolDenoms: []string{DenomOne, DenomTwo},
					},
					{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            defaultPoolID,
							TokenOutDenom: DenomFour,
						},
						PoolDenoms: []string{DenomTwo, DenomFour},
					},
				},
			},

			tokenInDenom: DenomOne,

			expectFiltered: true,
		},
		"not filtered: same pool id between routes": {
			routes: [][]usecase.CandidatePoolWrapper{
				{
					defaultDenomOneTwoOutTwoPool,
				},
				{
					defaultDenomOneTwoOutTwoPool,
				},
			},
			tokenInDenom: DenomOne,
		},
	}

	for name, tc := range tests {
		tc := tc
		s.Run(name, func() {

			filteredCandidateRoutes, err := routerusecase.ValidateAndFilterRoutes(tc.routes, tc.tokenInDenom, noOpLogger)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectError)
				return
			}
			s.Require().NoError(err)

			if tc.expectFiltered {
				s.Require().NotEqual(len(tc.routes), len(filteredCandidateRoutes.Routes))
				s.Require().Len(filteredCandidateRoutes.Routes, tc.expectFilteredRouteLength)
				return
			}

			s.Require().Equal(len(tc.routes), len(filteredCandidateRoutes.Routes))
		})
	}
}

// Validates that quotes constructed from mainnet state can be computed with no error
// for selected pairs.
func (s *RouterTestSuite) TestGetOptimalQuote_Mainnet() {
	tests := map[string]struct {
		tokenInDenom  string
		tokenOutDenom string

		amountIn osmomath.Int

		expectedRoutesCount int
	}{
		// This pair originally caused an error due to the lack of filtering that was
		// added later.
		"usdt for umee": {
			tokenInDenom:  USDT,
			tokenOutDenom: UMEE,

			amountIn: osmomath.NewInt(1000_000_000),

			expectedRoutesCount: 2,
		},
		"uosmo for uion": {
			tokenInDenom:  UOSMO,
			tokenOutDenom: UION,

			amountIn: osmomath.NewInt(5000000),

			expectedRoutesCount: 1,
		},
		"usdt for atom": {
			tokenInDenom:  USDT,
			tokenOutDenom: ATOM,

			amountIn: osmomath.NewInt(5000000),

			expectedRoutesCount: 1,
		},
		"uakt for umee": {
			tokenInDenom:  AKT,
			tokenOutDenom: UMEE,

			amountIn: osmomath.NewInt(100_000_000),

			expectedRoutesCount: 2,
		},
		// This test validates that with a greater max routes value, SQS is able to find
		// the path from umee to stOsmo
		"umee for stosmo": {
			tokenInDenom:  UMEE,
			tokenOutDenom: stOSMO,

			amountIn: osmomath.NewInt(1_000_000),

			expectedRoutesCount: 1,
		},

		"atom for akt": {
			tokenInDenom:  ATOM,
			tokenOutDenom: AKT,

			amountIn: osmomath.NewInt(1_000_000),

			expectedRoutesCount: 1,
		},
	}

	for name, tc := range tests {
		tc := tc
		s.Run(name, func() {
			// Setup mainnet router
			mainnetState := s.SetupMainnetState()

			// Mock router use case.
			mainnetUseCase := s.SetupRouterAndPoolsUsecase(mainnetState)

			// System under test
			quote, err := mainnetUseCase.Router.GetOptimalQuote(context.Background(), sdk.NewCoin(tc.tokenInDenom, tc.amountIn), tc.tokenOutDenom)

			// We only validate that error does not occur without actually validating the quote.
			s.Require().NoError(err)

			// TODO: update mainnet state and validate the quote for each test stricter.
			quoteRoutes := quote.GetRoute()
			s.Require().Len(quoteRoutes, tc.expectedRoutesCount)

			// Validate that the quote is not nil
			s.Require().NotNil(quote.GetAmountOut())
		})
	}
}

// Validates custom quotes for UOSMO to UION.
// That is, with the given pool ID, we expect the quote to be routed through the route
// that matches these pool IDs. Errors otherwise.
func (s *RouterTestSuite) TestGetCustomQuote_GetCustomDirectQuote_Mainnet_UOSMOUION() {
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
	poolsUsecase := poolsusecase.NewPoolsUsecase(&domain.PoolsConfig{}, "node-uri-placeholder", routerRepositoryMock)
	poolsUsecase.StorePools(mainnetState.Pools)

	routerUsecase := routerusecase.NewRouterUsecase(routerRepositoryMock, poolsUsecase, config, emptyCosmWasmPoolsRouterConfig, &log.NoOpLogger{}, cache.New(), cache.New())

	// This pool ID is second best: https://app.osmosis.zone/pool/2
	// The top one is https://app.osmosis.zone/pool/1110 which is not selected
	// due to custom parameter.
	const expectedPoolID = uint64(2)

	// System under test 2
	quote, err := routerUsecase.GetCustomDirectQuote(context.Background(), sdk.NewCoin(UOSMO, amountIn), UION, expectedPoolID)
	s.Require().NoError(err)
	s.validateExpectedPoolIDOneRouteOneHopQuote(quote, expectedPoolID)
}

// validates that the given quote has one route with one hop and the expected pool ID.
func (s *RouterTestSuite) validateExpectedPoolIDOneRouteOneHopQuote(quote domain.Quote, expectedPoolID uint64) {
	routes := quote.GetRoute()
	s.Require().Len(routes, 1)

	route := routes[0]

	routePools := route.GetPools()
	s.Require().Equal(1, len(routePools))
	s.Require().Equal(expectedPoolID, routePools[0].GetId())
}
