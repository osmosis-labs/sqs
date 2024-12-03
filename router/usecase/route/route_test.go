package route_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

type RouterTestSuite struct {
	routertesting.RouterTestHelper
}

func TestRouterTestSuite(t *testing.T) {
	suite.Run(t, new(RouterTestSuite))
}

var (
	// Concentrated liquidity constants
	ETH    = routertesting.ETH
	USDC   = routertesting.USDC
	USDT   = routertesting.USDT
	Denom0 = ETH
	Denom1 = USDC

	DefaultCurrentTick = routertesting.DefaultCurrentTick

	DefaultAmt0 = routertesting.DefaultAmt0
	DefaultAmt1 = routertesting.DefaultAmt1

	DefaultCoin0 = routertesting.DefaultCoin0
	DefaultCoin1 = routertesting.DefaultCoin1

	DefaultLiquidityAmt = routertesting.DefaultLiquidityAmt

	// router specific variables
	defaultTickModel = routertesting.DefaultTickModel

	noTakerFee = routertesting.NoTakerFee

	emptyRoute = routertesting.EmptyRoute
)

var (
	DefaultTakerFee     = routertesting.DefaultTakerFee
	DefaultPoolBalances = routertesting.DefaultPoolBalances
	DefaultSpreadFactor = routertesting.DefaultSpreadFactor

	DefaultPool = routertesting.DefaultPool
	EmptyRoute  = routertesting.EmptyRoute

	// Test denoms
	DenomOne   = routertesting.DenomOne
	DenomTwo   = routertesting.DenomTwo
	DenomThree = routertesting.DenomThree
	DenomFour  = routertesting.DenomFour
	DenomFive  = routertesting.DenomFive
	DenomSix   = routertesting.DenomSix
)

// This test validates that the pools in the route are converted into a new serializable
// type for clients with the following list of fields that are returned in each pool:
// - ID
// - Type
// - Balances
// - Spread Factor
// - Token Out Denom
// - Taker Fee
// Additionally, it validates that the method returns the spot price before swap and the
// effective spot price computed correctly.
// To achieve this, we set up a balancer and a transmuter pool.
// We estimate the balancer pool's spot prices at the beginning of the test.
// Transmuter is expected to have a spot price of one.
// Based on this fact and only having a testcase with a single balancer pool in the route,
// or balancer and trasnsmuter, we can validate that spot prices are computed equal to the
// spot price of the balancer pool.
func (s *RouterTestSuite) TestPrepareResultPools() {
	s.Setup()

	const (
		notCosmWasmPoolCodeID = 0
	)

	balancerPoolID := s.PrepareBalancerPoolWithCoins(sdk.NewCoins(
		sdk.NewCoin(DenomOne, osmomath.NewInt(2_000_000_000)),
		sdk.NewCoin(DenomTwo, osmomath.NewInt(1_000_000_000)),
	)...)

	pool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, balancerPoolID)
	s.Require().NoError(err)

	// Cast to balancer pool
	balancerPool, ok := pool.(*balancer.Pool)
	s.Require().True(ok)

	defaultTokenIn := sdk.NewCoin(DenomTwo, DefaultAmt0)

	// Estimate balancer pool spot price
	balancerPoolSpotPriceInOverOut, err := balancerPool.SpotPrice(sdk.Context{}, DenomOne, DenomTwo)
	s.Require().NoError(err)

	// Estimate balancer pool effective spot price
	expectedAmountOutBalancer, err := balancerPool.CalcOutAmtGivenIn(sdk.Context{}, sdk.NewCoins(defaultTokenIn), DenomOne, DefaultSpreadFactor)
	s.Require().NoError(err)
	expectedEffectivePriceBalancerInOverOut := expectedAmountOutBalancer.Amount.ToLegacyDec().Quo(defaultTokenIn.Amount.ToLegacyDec())

	// Prepare trasnmuter pool
	transmuter := s.PrepareCustomTransmuterPoolCustomProject(s.TestAccs[0], []string{DenomOne, DenomThree}, "sqs", "scripts")

	testcases := map[string]struct {
		tokenIn sdk.Coin

		route route.RouteImpl

		expectedPools []domain.RoutablePool

		expectedSpotPriceInBaseOutQuote osmomath.Dec

		expectedEffectiveSpotPriceInOverOut osmomath.Dec
	}{
		"empty route": {
			tokenIn: defaultTokenIn,

			route: emptyRoute,

			expectedPools: []domain.RoutablePool{},

			expectedSpotPriceInBaseOutQuote:     osmomath.OneDec(),
			expectedEffectiveSpotPriceInOverOut: osmomath.OneDec(),
		},
		"single balancer pool in route": {
			tokenIn: defaultTokenIn,

			route: WithRoutePools(
				emptyRoute,
				[]domain.RoutablePool{
					mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultPool, DenomOne), balancerPool),
				},
			),

			expectedPools: []domain.RoutablePool{
				pools.NewRoutableResultPool(
					balancerPoolID,
					poolmanagertypes.Balancer,
					DefaultSpreadFactor,
					DenomOne,
					DefaultTakerFee,
					notCosmWasmPoolCodeID,
				),
			},

			// Balancer is the only pool in the route so its spot price is expected.
			expectedSpotPriceInBaseOutQuote:     balancerPoolSpotPriceInOverOut.Dec(),
			expectedEffectiveSpotPriceInOverOut: expectedEffectivePriceBalancerInOverOut,
		},
		"balancer and transmuter in route": {
			tokenIn: defaultTokenIn,

			route: WithRoutePools(
				emptyRoute,
				[]domain.RoutablePool{
					mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultPool, DenomOne), balancerPool),
					mocks.WithChainPoolModel(mocks.WithTokenOutDenom(DefaultPool, DenomThree), transmuter),
				},
			),

			expectedPools: []domain.RoutablePool{
				pools.NewRoutableResultPool(
					balancerPoolID,
					poolmanagertypes.Balancer,
					DefaultSpreadFactor,
					DenomOne,
					DefaultTakerFee,
					notCosmWasmPoolCodeID,
				),
				pools.NewRoutableResultPool(
					transmuter.GetId(),
					poolmanagertypes.CosmWasm,
					DefaultSpreadFactor,
					DenomThree,
					DefaultTakerFee,
					transmuter.GetCodeId(),
				),
			},

			// Transmuter has spot price of one. Therefore, the spot price of the route
			// should be the same as the spot price of the balancer pool.
			expectedSpotPriceInBaseOutQuote:     balancerPoolSpotPriceInOverOut.Dec(),
			expectedEffectiveSpotPriceInOverOut: expectedEffectivePriceBalancerInOverOut,
		},
	}

	for name, tc := range testcases {
		tc := tc
		s.Run(name, func() {

			// Note: token in is chosen arbitrarily since it is irrelevant for this test
			actualPools, spotPriceBeforeInBaseOutQuote, _, err := tc.route.PrepareResultPools(context.TODO(), tc.tokenIn, &log.NoOpLogger{})
			s.Require().NoError(err)

			s.Require().Equal(tc.expectedSpotPriceInBaseOutQuote, spotPriceBeforeInBaseOutQuote)

			s.ValidateRoutePools(tc.expectedPools, actualPools)
		})
	}
}

func WithRoutePools(r route.RouteImpl, pools []domain.RoutablePool) route.RouteImpl {
	return routertesting.WithRoutePools(r, pools)
}
