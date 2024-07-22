package usecase_test

import (
	"context"
	"encoding/json"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v25/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v25/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

var (
	defaultAmount                 = sdk.NewInt(100_000_00)
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
// Route 1: 2 hop
// Route 2: 1 hop
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

		poolOneBalances = sdk.NewCoins(
			sdk.NewCoin(USDT, defaultAmount.MulRaw(5)),
			sdk.NewCoin(ETH, defaultAmount),
		)

		poolTwoBalances = sdk.NewCoins(
			sdk.NewCoin(USDC, defaultAmount),
			sdk.NewCoin(USDT, defaultAmount),
		)

		poolThreeBalances = sdk.NewCoins(
			sdk.NewCoin(ETH, defaultAmount),
			sdk.NewCoin(USDC, defaultAmount.MulRaw(4)),
		)
	)

	// Prepare 2 routes
	// Route 1: 2 hops
	// Route 2: 1 hop

	// Pool USDT / ETH -> 0.01 spread factor & 5 USDTfor 1 ETH
	poolIDOne := s.PrepareCustomBalancerPool([]balancer.PoolAsset{
		{
			Token:  sdk.NewCoin(USDT, defaultAmount.MulRaw(5)),
			Weight: sdk.NewInt(100),
		},
		{
			Token:  sdk.NewCoin(ETH, defaultAmount),
			Weight: sdk.NewInt(100),
		},
	}, balancer.PoolParams{
		SwapFee: sdk.NewDecWithPrec(1, 2),
		ExitFee: osmomath.ZeroDec(),
	})

	poolOne, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, poolIDOne)
	s.Require().NoError(err)

	// Pool USDC / USDT -> 0.01 spread factor & 1 USDC for 1 USDT
	poolIDTwo := s.PrepareCustomBalancerPool([]balancer.PoolAsset{
		{
			Token:  sdk.NewCoin(USDC, defaultAmount),
			Weight: sdk.NewInt(100),
		},
		{
			Token:  sdk.NewCoin(USDT, defaultAmount),
			Weight: sdk.NewInt(100),
		},
	}, balancer.PoolParams{
		SwapFee: sdk.NewDecWithPrec(3, 2),
		ExitFee: osmomath.ZeroDec(),
	})

	poolTwo, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, poolIDTwo)
	s.Require().NoError(err)

	// Pool ETH / USDC -> 0.005 spread factor & 4 USDC for 1 ETH
	poolIDThree := s.PrepareCustomBalancerPool([]balancer.PoolAsset{
		{
			Token:  sdk.NewCoin(ETH, defaultAmount),
			Weight: sdk.NewInt(100),
		},
		{
			Token:  sdk.NewCoin(USDC, defaultAmount.MulRaw(4)),
			Weight: sdk.NewInt(100),
		},
	}, balancer.PoolParams{
		SwapFee: sdk.NewDecWithPrec(5, 3),
		ExitFee: osmomath.ZeroDec(),
	})

	poolThree, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, poolIDThree)
	s.Require().NoError(err)

	testcases := []struct {
		name  string
		quote domain.Quote

		expectedRoutes       []domain.SplitRoute
		expectedEffectiveFee string
		expectedJSON         string
	}{
		{
			name: "exact amount in",
			quote: &usecase.QuoteImpl{
				AmountIn:  sdk.NewCoin(ETH, totalInAmount),
				AmountOut: totalOutAmount,

				// 2 routes with 50-50 split, each single hop
				Route: []domain.SplitRoute{

					// Route 1
					&usecase.RouteWithOutAmount{
						RouteImpl: route.RouteImpl{
							Pools: []domain.RoutablePool{
								s.newRoutablePool(
									sqsdomain.NewPool(poolOne, poolOne.GetSpreadFactor(sdk.Context{}), poolOneBalances),
									USDT,
									takerFeeOne,
									domain.CosmWasmPoolRouterConfig{},
								),
								s.newRoutablePool(
									sqsdomain.NewPool(poolTwo, poolTwo.GetSpreadFactor(sdk.Context{}), poolTwoBalances),
									USDC,
									takerFeeTwo,
									domain.CosmWasmPoolRouterConfig{},
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
								s.newRoutablePool(
									sqsdomain.NewPool(poolThree, poolThree.GetSpreadFactor(sdk.Context{}), poolThreeBalances),
									USDC,
									takerFeeThree,
									domain.CosmWasmPoolRouterConfig{},
								),
							},
						},

						InAmount:  totalInAmount.QuoRaw(2),
						OutAmount: totalOutAmount.QuoRaw(2),
					},
				},
				EffectiveFee: osmomath.ZeroDec(),
			},
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
			expectedJSON: `{
			  "amount_in": {
				"denom": "ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5",
				"amount": "10000000"
			  },
			  "amount_out": "40000000",
			  "route": [
				{
				  "pools": [
					{
					  "id": 1,
					  "type": 0,
					  "balances": [],
					  "spread_factor": "0.010000000000000000",
					  "token_out_denom": "ibc/4ABBEF4C8926DDDB320AE5188CFD63267ABBCEFC0583E4AE05D6E5AA2401DDAB",
					  "taker_fee": "0.020000000000000000"
					},
					{
					  "id": 2,
					  "type": 0,
					  "balances": [],
					  "spread_factor": "0.030000000000000000",
					  "token_out_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
					  "taker_fee": "0.000400000000000000"
					}
				  ],
				  "has-cw-pool": false,
				  "out_amount": "20000000",
				  "in_amount": "5000000"
				},
				{
				  "pools": [
					{
					  "id": 3,
					  "type": 0,
					  "balances": [],
					  "spread_factor": "0.005000000000000000",
					  "token_out_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
					  "taker_fee": "0.003000000000000000"
					}
				  ],
				  "has-cw-pool": false,
				  "out_amount": "20000000",
				  "in_amount": "5000000"
				}
			  ],
			  "effective_fee": "0.011696000000000000",
			  "price_impact": "-0.565353638051463862",
			  "in_base_out_quote_spot_price": "4.500000000000000000"
			}`,
		},
		{
			name: "exact amount out",
			quote: usecase.NewQuoteExactAmountOut(
				&usecase.QuoteImpl{

					AmountIn:  sdk.NewCoin(ETH, totalInAmount),
					AmountOut: totalOutAmount,

					// 2 routes with 50-50 split, each single hop
					Route: []domain.SplitRoute{
						&usecase.RouteWithOutAmount{
							RouteImpl: route.RouteImpl{
								Pools: []domain.RoutablePool{
									s.newRoutablePool(
										sqsdomain.NewPool(poolOne, poolOne.GetSpreadFactor(sdk.Context{}), poolOneBalances),
										USDT,
										takerFeeOne,
										domain.CosmWasmPoolRouterConfig{},
									),
									s.newRoutablePool(
										sqsdomain.NewPool(poolTwo, poolTwo.GetSpreadFactor(sdk.Context{}), poolTwoBalances),
										USDC,
										takerFeeTwo,
										domain.CosmWasmPoolRouterConfig{},
									),
								},
							},

							InAmount:  totalInAmount.QuoRaw(2),
							OutAmount: totalOutAmount.QuoRaw(3),
						},
						&usecase.RouteWithOutAmount{
							RouteImpl: route.RouteImpl{
								Pools: []domain.RoutablePool{
									s.newRoutablePool(
										sqsdomain.NewPool(poolThree, poolThree.GetSpreadFactor(sdk.Context{}), poolThreeBalances),
										USDC,
										takerFeeThree,
										domain.CosmWasmPoolRouterConfig{},
									),
								},
							},

							InAmount:  totalInAmount.QuoRaw(4),
							OutAmount: totalOutAmount.QuoRaw(5),
						},
					},
					EffectiveFee: osmomath.ZeroDec(),
				},
			),
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
			expectedJSON: `{
			  "amount_in": "40000000",
			  "amount_out": {
				"denom": "ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5",
				"amount": "10000000"
			  },
			  "route": [
				{
				  "pools": [
					{
					  "id": 1,
					  "type": 0,
					  "balances": [],
					  "spread_factor": "0.010000000000000000",
					  "token_in_denom": "ibc/4ABBEF4C8926DDDB320AE5188CFD63267ABBCEFC0583E4AE05D6E5AA2401DDAB",
					  "taker_fee": "0.020000000000000000"
					},
					{
					  "id": 2,
					  "type": 0,
					  "balances": [],
					  "spread_factor": "0.030000000000000000",
					  "token_in_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
					  "taker_fee": "0.000400000000000000"
					}
				  ],
				  "has-cw-pool": false,
				  "out_amount": "5000000",
				  "in_amount": "13333333"
				},
				{
				  "pools": [
					{
					  "id": 3,
					  "type": 0,
					  "balances": [],
					  "spread_factor": "0.005000000000000000",
					  "token_in_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
					  "taker_fee": "0.003000000000000000"
					}
				  ],
				  "has-cw-pool": false,
				  "out_amount": "2500000",
				  "in_amount": "8000000"
				}
			  ],
			  "effective_fee": "0.010946000000000000",
			  "price_impact": "-0.593435820925030124",
			  "in_base_out_quote_spot_price": "3.500000000000000000"
							}`,
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			// System under test
			routes, effectiveFee, err := tc.quote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor)
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
			Weight: sdk.NewInt(100),
		},
		{
			Token:  sdk.NewCoin(USDC, defaultAmount.MulRaw(4)),
			Weight: sdk.NewInt(100),
		},
	}, balancer.PoolParams{
		SwapFee: sdk.NewDecWithPrec(5, 3),
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
	testQuote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor)

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
	cosmWasmPoolsParams := pools.CosmWasmPoolsParams{
		Config:                cosmWasmConfig,
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(pool, tokenOutDenom, takerFee, cosmWasmPoolsParams)
	s.Require().NoError(err)
	return routablePool
}
