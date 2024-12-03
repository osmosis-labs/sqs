package usecase_test

import (
	"context"
	"encoding/json"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

var (
	defaultAmount                 = osmomath.NewInt(100_000_00)
	totalInAmount                 = defaultAmount
	totalOutAmount                = defaultAmount.MulRaw(4)
	defaultSpotPriceScalingFactor = osmomath.OneDec()
)

// TestPrepareResult prepares the result of the quote for output to the client.
// First, it strips away unnecessary fields from each pool in the route.
// Additionally, it computes the effective spread factor from all routes.
//
// The test structure is as follows:
// - Set up a 50-50 split route
// - Route 1: 2 hop
// - Route 2: 1 hop
//
// Validate that the effective swap fee is computed correctly.
func (s *RouterTestSuite) TestPrepareResult() {
	s.SetupTest()

	const (
		notCosmWasmPoolCodeID = 0
	)

	var (
		takerFeeOne   = osmomath.NewDecWithPrec(2, 2)
		takerFeeTwo   = osmomath.NewDecWithPrec(4, 4)
		takerFeeThree = osmomath.NewDecWithPrec(3, 3)
	)

	// Prepare 2 routes
	// Route 1: 2 hops
	// Route 2: 1 hop

	// Pool USDT / ETH -> 0.01 spread factor & 5 USDTfor 1 ETH
	poolIDOne, poolOne := s.PoolOne()

	// Pool USDC / USDT -> 0.01 spread factor & 1 USDC for 1 USDT
	poolIDTwo, poolTwo := s.PoolTwo()

	// Pool ETH / USDC -> 0.005 spread factor & 4 USDC for 1 ETH
	poolIDThree, poolThree := s.PoolThree()

	testcases := []struct {
		name  string
		quote domain.Quote

		expectedRoutes       []domain.SplitRoute
		expectedEffectiveFee string
		expectedJSON         string
	}{
		{
			name:  "exact amount in",
			quote: s.NewExactAmountInQuote(poolOne, poolTwo, poolThree),
			expectedRoutes: []domain.SplitRoute{
				// Route 1
				&usecase.RouteWithOutAmount{
					RouteImpl: route.RouteImpl{
						Pools: []domain.RoutablePool{
							pools.NewRoutableResultPool(
								poolIDOne,
								poolmanagertypes.Balancer,
								poolOne.GetSpreadFactor(sdk.Context{}),
								USDT,
								takerFeeOne,
								notCosmWasmPoolCodeID,
							),
							pools.NewRoutableResultPool(
								poolIDTwo,
								poolmanagertypes.Balancer,
								poolTwo.GetSpreadFactor(sdk.Context{}),
								USDC,
								takerFeeTwo,
								notCosmWasmPoolCodeID,
							),
						},
					},

					InAmount:  totalInAmount.QuoRaw(2),
					OutAmount: totalOutAmount.QuoRaw(2),
				},

				// Route 2
				&usecase.RouteWithOutAmount{
					RouteImpl: route.RouteImpl{
						Pools: []domain.RoutablePool{
							pools.NewRoutableResultPool(
								poolIDThree,
								poolmanagertypes.Balancer,
								poolThree.GetSpreadFactor(sdk.Context{}),
								USDC,
								takerFeeThree,
								notCosmWasmPoolCodeID,
							),
						},
					},

					InAmount:  totalInAmount.QuoRaw(2),
					OutAmount: totalOutAmount.QuoRaw(2),
				},
			},
			// (0.02 + (1 - 0.02) * 0.0004) * 0.5 + 0.003 * 0.5
			expectedEffectiveFee: "0.011696000000000000",
			expectedJSON:         s.MustReadFile("./routertesting/parsing/quote_amount_in_response.json"),
		},
		{
			name:  "exact amount out",
			quote: s.NewExactAmountOutQuote(poolOne, poolTwo, poolThree),
			expectedRoutes: []domain.SplitRoute{
				&usecase.RouteWithOutAmount{
					RouteImpl: route.RouteImpl{
						Pools: []domain.RoutablePool{
							pools.NewExactAmountOutRoutableResultPool(
								poolIDOne,
								poolmanagertypes.Balancer,
								poolOne.GetSpreadFactor(sdk.Context{}),
								USDT,
								takerFeeOne,
								notCosmWasmPoolCodeID,
							),
							pools.NewExactAmountOutRoutableResultPool(
								poolIDTwo,
								poolmanagertypes.Balancer,
								poolTwo.GetSpreadFactor(sdk.Context{}),
								USDC,
								takerFeeTwo,
								notCosmWasmPoolCodeID,
							),
						},
					},

					InAmount:  totalOutAmount.QuoRaw(3),
					OutAmount: totalInAmount.QuoRaw(2),
				},
				&usecase.RouteWithOutAmount{
					RouteImpl: route.RouteImpl{
						Pools: []domain.RoutablePool{
							pools.NewExactAmountOutRoutableResultPool(
								poolIDThree,
								poolmanagertypes.Balancer,
								poolThree.GetSpreadFactor(sdk.Context{}),
								USDC,
								takerFeeThree,
								notCosmWasmPoolCodeID,
							),
						},
					},

					InAmount:  totalOutAmount.QuoRaw(5),
					OutAmount: totalInAmount.QuoRaw(4),
				},
			},
			expectedEffectiveFee: "0.010946000000000000",
			expectedJSON:         s.MustReadFile("./routertesting/parsing/quote_amount_out_response.json"),
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			// System under test
			routes, effectiveFee, err := tc.quote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor, &log.NoOpLogger{})
			s.Require().NoError(err)

			// Validate JSON representation, which is used for output to the client
			// That covers amount in and amount out which can not be validated with getter methods.
			response, err := json.Marshal(tc.quote)
			s.Require().NoError(err)
			s.Require().JSONEq(tc.expectedJSON, string(response))

			// Validate routes.
			s.validateRoutes(tc.expectedRoutes, routes)
			s.validateRoutes(tc.expectedRoutes, tc.quote.GetRoute())

			// Validate effective spread factor.
			s.Require().Equal(tc.expectedEffectiveFee, effectiveFee.String())
			s.Require().Equal(tc.expectedEffectiveFee, tc.quote.GetEffectiveFee().String())
		})
	}
}

// This test validates that price impact is computed correctly.
func (s *RouterTestSuite) TestPrepareResult_PriceImpact() {
	s.Setup()

	// Pool ETH / USDC -> 0.005 spread factor & 4 USDC for 1 ETH
	poolID := s.PrepareCustomBalancerPool([]balancer.PoolAsset{
		{
			Token:  sdk.NewCoin(ETH, defaultAmount),
			Weight: osmomath.NewInt(100),
		},
		{
			Token:  sdk.NewCoin(USDC, defaultAmount.MulRaw(4)),
			Weight: osmomath.NewInt(100),
		},
	}, balancer.PoolParams{
		SwapFee: osmomath.NewDecWithPrec(5, 3),
		ExitFee: osmomath.ZeroDec(),
	})

	poolOne, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, poolID)
	s.Require().NoError(err)

	// Compute spot price before swap
	spotPriceInBaseOutQuote, err := poolOne.SpotPrice(sdk.Context{}, USDC, ETH)
	s.Require().NoError(err)

	coinIn := sdk.NewCoin(ETH, totalInAmount)

	// Compute expected effective price
	tokenInAfterFee, _ := poolmanager.CalcTakerFeeExactIn(coinIn, DefaultTakerFee)
	expectedEffectivePrice := totalOutAmount.ToLegacyDec().Quo(tokenInAfterFee.Amount.ToLegacyDec())

	// Compute expected price impact
	expectedPriceImpact := expectedEffectivePrice.Quo(spotPriceInBaseOutQuote.Dec()).Sub(osmomath.OneDec())

	// Setup quote
	testQuote := &usecase.QuoteImpl{
		AmountIn:  sdk.NewCoin(ETH, totalInAmount),
		AmountOut: totalOutAmount,

		// 2 routes with 50-50 split, each single hop
		Route: []domain.SplitRoute{

			// Route 1
			&usecase.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						mocks.WithMockedTokenOut(
							mocks.WithTokenOutDenom(
								mocks.WithChainPoolModel(DefaultMockPool, poolOne), USDC),
							sdk.NewCoin(USDC, totalOutAmount),
						),
					},
				},

				InAmount:  totalInAmount,
				OutAmount: totalOutAmount,
			},
		},
		EffectiveFee: osmomath.ZeroDec(),
	}

	// System under test.
	testQuote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor, &log.NoOpLogger{})

	// Validate price impact.
	s.Require().Equal(expectedPriceImpact.String(), testQuote.GetPriceImpact().String())
}

// validateRoutes validates that the given routes are equal.
// Specifically, validates:
// - Pools
// - In amount
// - Out amount
func (s *RouterTestSuite) validateRoutes(expectedRoutes []domain.SplitRoute, actualRoutes []domain.SplitRoute) {
	s.Require().Equal(len(expectedRoutes), len(actualRoutes))
	for i, expectedRoute := range expectedRoutes {
		actualRoute := actualRoutes[i]

		// Validate pools
		s.ValidateRoutePools(expectedRoute.GetPools(), actualRoute.GetPools())

		// Validate in amount
		s.Require().Equal(expectedRoute.GetAmountIn().String(), actualRoute.GetAmountIn().String())

		// Validate out amount
		s.Require().Equal(expectedRoute.GetAmountOut().String(), actualRoute.GetAmountOut().String())
	}
}

func (s *RouterTestSuite) newRoutablePool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec, cosmWasmConfig domain.CosmWasmPoolRouterConfig) domain.RoutablePool {
	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		Config:                cosmWasmConfig,
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(pool, tokenOutDenom, takerFee, cosmWasmPoolsParams)
	s.Require().NoError(err)
	return routablePool
}
