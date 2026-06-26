package usecase_test

import (
	"context"
	"encoding/json"

	sdk "github.com/cosmos/cosmos-sdk/types"

	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"

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
		name                 string
		quote                domain.Quote
		tokenMetadataFetcher domain.TokensMetadataFetcher

		expectedRoutes       []domain.SplitRoute
		expectedEffectiveFee string
		expectedJSON         string
	}{
		{
			name:  "exact amount in",
			quote: s.NewExactAmountInQuote(poolOne, poolTwo, poolThree),
			tokenMetadataFetcher: &mocks.TokensUsecaseMock{
				GetPoolDenomsMetadataFunc: func(chainDenoms []string) domain.PoolDenomMetaDataMap {
					return domain.PoolDenomMetaDataMap{
						"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4": {TotalLiquidityCap: osmomath.NewInt(51951659)},
						"ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5": {TotalLiquidityCap: osmomath.NewInt(10000000)},
						"ibc/4ABBEF4C8926DDDB320AE5188CFD63267ABBCEFC0583E4AE05D6E5AA2401DDAB": {TotalLiquidityCap: osmomath.NewInt(53)},
					}
				},
			},
			expectedRoutes: []domain.SplitRoute{
				// Route 1
				&route.RouteWithOutAmount{
					RouteImpl: route.RouteImpl{
						Pools: []domain.RoutablePool{
							pools.NewRoutableResultPool(
								poolIDOne,
								poolmanagertypes.Balancer,
								poolOne.GetSpreadFactor(sdk.Context{}),
								USDT,
								takerFeeOne,
								s.PoolOneLiquidityCap(),
								notCosmWasmPoolCodeID,
							),
							pools.NewRoutableResultPool(
								poolIDTwo,
								poolmanagertypes.Balancer,
								poolTwo.GetSpreadFactor(sdk.Context{}),
								USDC,
								takerFeeTwo,
								s.PoolTwoLiquidityCap(),
								notCosmWasmPoolCodeID,
							),
						},
					},

					InAmount:  totalInAmount.QuoRaw(2),
					OutAmount: totalOutAmount.QuoRaw(2),
				},

				// Route 2
				&route.RouteWithOutAmount{
					RouteImpl: route.RouteImpl{
						Pools: []domain.RoutablePool{
							pools.NewRoutableResultPool(
								poolIDThree,
								poolmanagertypes.Balancer,
								poolThree.GetSpreadFactor(sdk.Context{}),
								USDC,
								takerFeeThree,
								s.PoolThreeLiquidityCap(),
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
			tokenMetadataFetcher: &mocks.TokensUsecaseMock{
				GetPoolDenomsMetadataFunc: func(chainDenoms []string) domain.PoolDenomMetaDataMap {
					return domain.PoolDenomMetaDataMap{
						"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4": {TotalLiquidityCap: osmomath.NewInt(851696596)},
						"ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5": {TotalLiquidityCap: osmomath.NewInt(5119159)},
						"ibc/4ABBEF4C8926DDDB320AE5188CFD63267ABBCEFC0583E4AE05D6E5AA2401DDAB": {TotalLiquidityCap: osmomath.NewInt(851951)},
					}
				},
			},
			expectedRoutes: []domain.SplitRoute{
				&route.RouteWithOutAmount{
					RouteImpl: route.RouteImpl{
						Pools: []domain.RoutablePool{
							pools.NewExactAmountOutRoutableResultPool(
								poolIDOne,
								poolmanagertypes.Balancer,
								poolOne.GetSpreadFactor(sdk.Context{}),
								USDT,
								takerFeeOne,
								s.PoolOneLiquidityCap(),
								notCosmWasmPoolCodeID,
							),
							pools.NewExactAmountOutRoutableResultPool(
								poolIDTwo,
								poolmanagertypes.Balancer,
								poolTwo.GetSpreadFactor(sdk.Context{}),
								USDC,
								takerFeeTwo,
								s.PoolTwoLiquidityCap(),
								notCosmWasmPoolCodeID,
							),
						},
					},

					InAmount:  totalOutAmount.QuoRaw(3),
					OutAmount: totalInAmount.QuoRaw(2),
				},
				&route.RouteWithOutAmount{
					RouteImpl: route.RouteImpl{
						Pools: []domain.RoutablePool{
							pools.NewExactAmountOutRoutableResultPool(
								poolIDThree,
								poolmanagertypes.Balancer,
								poolThree.GetSpreadFactor(sdk.Context{}),
								USDC,
								takerFeeThree,
								s.PoolThreeLiquidityCap(),
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
			routes, effectiveFee, err := tc.quote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor, nil, tc.tokenMetadataFetcher, &log.NoOpLogger{})
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
	spotPriceInBaseOutQuote, err := poolOne.SpotPrice(s.Ctx, USDC, ETH)
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
			&route.RouteWithOutAmount{
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
	testQuote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor, nil, nil, &log.NoOpLogger{})

	// Validate price impact.
	s.Require().Equal(expectedPriceImpact.String(), testQuote.GetPriceImpact().String())
}

// TestPrepareResult_ExactOut_TruePath_EffectiveFee validates the effective-fee
// computation on the *true* exact-out (in-given-out) PrepareResult path, i.e. the
// branch taken when the embedded quoteExactAmountIn is nil (as produced by
// estimateAndRankSingleRouteQuoteInGivenOut after the exact-out wiring).
//
// This is distinct from TestPrepareResult's "exact amount out" case, which sets the
// embedded quoteExactAmountIn and therefore exercises the legacy inversion fallback.
//
// Regression intent: the effective fee must be the per-route compounded taker fee
// (1 - prod(1 - poolFee_i)) weighted by each route's share of the total output. It must
// reflect the actual pool taker fees, not a value inherited from the opposite swap
// direction (the pool-1925 metadata-corruption class of bug).
//
// Split:
//   - Route 1 (2 hops: takerFeeOne=0.02, takerFeeTwo=0.0004) -> compounded 0.020392
//   - Route 2 (1 hop:  takerFeeThree=0.003)                  -> compounded 0.003
//   - Output split 3:2 (fractions 0.6 / 0.4)
//
// Expected effective fee = 0.020392*0.6 + 0.003*0.4 = 0.0134352
func (s *RouterTestSuite) TestPrepareResult_ExactOut_TruePath_EffectiveFee() {
	s.SetupTest()

	_, poolThree := s.PoolThree()

	var (
		// Two single-hop routes (ETH -> USDC) with distinct taker fees, output split 3:2.
		routeOneFee = osmomath.MustNewDecFromStr("0.02")
		routeTwoFee = osmomath.MustNewDecFromStr("0.003")

		amountOut   = sdk.NewCoin(USDC, osmomath.NewInt(5_000_000))
		routeOneOut = osmomath.NewInt(3_000_000)
		routeTwoOut = osmomath.NewInt(2_000_000)

		// Input amounts are arbitrary here (fee math depends only on taker fees and
		// the output-share weighting); their sum is reported as AmountIn.
		routeOneIn = osmomath.NewInt(750_000)
		routeTwoIn = osmomath.NewInt(500_000)
		amountIn   = routeOneIn.Add(routeTwoIn)
	)

	quote := s.NewExactAmountOutTrueQuote(
		poolThree,
		amountIn, amountOut,
		routeOneFee, routeTwoFee,
		routeOneIn, routeOneOut,
		routeTwoIn, routeTwoOut,
	)

	// System under test. tokenMetadataFetcher is nil: Tokens enrichment is not under test here.
	_, effectiveFee, err := quote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor, nil, nil, &log.NoOpLogger{})
	s.Require().NoError(err)

	// Single-hop routes, so each route's fee is its pool taker fee; weighted by output share:
	// 0.02 * 0.6 + 0.003 * 0.4 = 0.0132.
	const expectedEffectiveFee = "0.013200000000000000"
	s.Require().Equal(expectedEffectiveFee, effectiveFee.String())
	s.Require().Equal(expectedEffectiveFee, quote.GetEffectiveFee().String())
}

// TestPrepareResult_ExactOut_TruePath_PriceImpact validates the price-impact sign and
// formula on the true exact-out path.
//
// Regression intent: the exact-out wiring computes price impact as
// (effectiveOutPerIn / spotOutPerIn) - 1, which is NEGATIVE when the trade is adverse
// (effective execution worse than spot). A non-negative or zero result here would mean
// the sign convention regressed relative to the out-given-in path, which the frontend
// slippage logic (computeSuggestedSlippage / outputDifference) relies on.
func (s *RouterTestSuite) TestPrepareResult_ExactOut_TruePath_PriceImpact() {
	s.SetupTest()

	_, poolThree := s.PoolThree()

	var (
		routeOneFee = osmomath.MustNewDecFromStr("0.02")
		routeTwoFee = osmomath.MustNewDecFromStr("0.003")

		amountOut   = sdk.NewCoin(USDC, osmomath.NewInt(5_000_000))
		routeOneOut = osmomath.NewInt(3_000_000)
		routeTwoOut = osmomath.NewInt(2_000_000)
		routeOneIn  = osmomath.NewInt(750_000)
		routeTwoIn  = osmomath.NewInt(500_000)
		amountIn    = routeOneIn.Add(routeTwoIn)
	)

	quote := s.NewExactAmountOutTrueQuote(
		poolThree,
		amountIn, amountOut,
		routeOneFee, routeTwoFee,
		routeOneIn, routeOneOut,
		routeTwoIn, routeTwoOut,
	)

	_, _, err := quote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor, nil, nil, &log.NoOpLogger{})
	s.Require().NoError(err)

	priceImpact := quote.GetPriceImpact()
	spotInPerOut := quote.GetInBaseOutQuoteSpotPrice()

	// Both must be populated on the true exact-out path.
	s.Require().False(priceImpact.IsNil(), "price impact should be populated on the true exact-out path")
	s.Require().False(spotInPerOut.IsNil(), "in-base-out-quote spot price should be populated")
	s.Require().True(spotInPerOut.IsPositive(), "spot price (in per out) must be positive, got %s", spotInPerOut.String())

	// Re-derive price impact from the quote's own reported spot price and amounts to validate
	// the wiring's formula end to end, independent of the pool's internal spot math:
	//   InBaseOutQuoteSpotPrice = spotInPerOut = 1 / spotOutPerIn
	//   PriceImpact             = effectiveOutPerIn / spotOutPerIn - 1
	//                           = (amountOut/amountIn) * spotInPerOut - 1
	//
	// The SUT computes price impact via a slightly different chain of Quo/Mul/Sub ops than
	// this re-derivation, so the two can differ in the last few decimal places due to
	// osmomath rounding. Assert closeness within a small tolerance rather than exact
	// equality; the sign convention below is the regression-critical property.
	effectiveOutPerIn := amountOut.Amount.ToLegacyDec().Quo(amountIn.ToLegacyDec())
	expectedPriceImpact := effectiveOutPerIn.Mul(spotInPerOut).Sub(osmomath.OneDec())

	priceImpactDiff := priceImpact.Sub(expectedPriceImpact).Abs()
	tolerance := osmomath.MustNewDecFromStr("0.0000001")
	s.Require().True(priceImpactDiff.LTE(tolerance),
		"price impact %s must be within %s of (amountOut/amountIn)*spotInPerOut - 1 = %s",
		priceImpact.String(), tolerance.String(), expectedPriceImpact.String())

	// Sign convention: an adverse trade (effective execution worse than spot) yields a
	// negative price impact. With effectiveOutPerIn < spotOutPerIn the product
	// effectiveOutPerIn*spotInPerOut < 1, so the impact is negative. Assert the sign matches
	// the adverse/benign relationship rather than hardcoding pool spot output.
	spotOutPerIn := osmomath.OneDec().Quo(spotInPerOut)
	if effectiveOutPerIn.LT(spotOutPerIn) {
		s.Require().True(priceImpact.IsNegative(),
			"exact-out price impact must be negative when adverse, got %s", priceImpact.String())
	}
}

// TestPrepareResult_ExactOut_TruePath_TakerFeeMetadata is the pool-1925 regression:
// a pool that charges zero taker fee in one direction must also report zero taker fee
// in the exact-out direction. The previous inversion approach could surface a taker fee
// from the opposite direction, corrupting the fee metadata.
//
// It builds a single-route, single-hop exact-out quote whose only pool charges zero
// taker fee and asserts the effective fee is exactly zero, then contrasts it with a
// non-zero-fee pool to confirm the fee is actually sourced from the pool (not a constant).
func (s *RouterTestSuite) TestPrepareResult_ExactOut_TruePath_TakerFeeMetadata() {
	s.SetupTest()

	_, poolThree := s.PoolThree()

	var (
		amountOut = sdk.NewCoin(USDC, osmomath.NewInt(4_000_000))
		amountIn  = osmomath.NewInt(1_000_000)

		// Wrap the chain pool as an SQS pool so it satisfies ingesttypes.PoolI.
		sqsPoolThree = ingesttypes.NewPool(poolThree, ingesttypes.SQSPool{
			SpreadFactor:     poolThree.GetSpreadFactor(sdk.Context{}),
			PoolLiquidityCap: osmomath.NewInt(719_951),
		})
	)

	// Single hop, single route through poolThree (ETH -> USDC), zero taker fee.
	zeroFeeQuote := &usecase.QuoteExactAmountOut{
		AmountIn:  amountIn,
		AmountOut: amountOut,
		Route: []domain.SplitRoute{
			&route.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(sqsPoolThree, ETH, USDC, osmomath.ZeroDec(), emptyCosmWasmPoolsRouterConfig),
					},
				},
				InAmount:  amountIn,
				OutAmount: amountOut.Amount,
			},
		},
		EffectiveFee: osmomath.ZeroDec(),
	}

	_, zeroFee, err := zeroFeeQuote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor, nil, nil, &log.NoOpLogger{})
	s.Require().NoError(err)
	s.Require().Equal("0.000000000000000000", zeroFee.String(),
		"a zero-taker-fee pool must report zero effective fee on the exact-out path")

	// Contrast: the same single hop with a non-zero taker fee must report exactly that fee,
	// confirming the effective fee is sourced from the pool's taker fee.
	const nonZeroFeeStr = "0.003000000000000000"
	nonZeroFeeQuote := &usecase.QuoteExactAmountOut{
		AmountIn:  amountIn,
		AmountOut: amountOut,
		Route: []domain.SplitRoute{
			&route.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(sqsPoolThree, ETH, USDC, osmomath.MustNewDecFromStr("0.003"), emptyCosmWasmPoolsRouterConfig),
					},
				},
				InAmount:  amountIn,
				OutAmount: amountOut.Amount,
			},
		},
		EffectiveFee: osmomath.ZeroDec(),
	}

	_, nonZeroFee, err := nonZeroFeeQuote.PrepareResult(context.TODO(), defaultSpotPriceScalingFactor, nil, nil, &log.NoOpLogger{})
	s.Require().NoError(err)
	s.Require().Equal(nonZeroFeeStr, nonZeroFee.String())
	s.Require().True(nonZeroFee.GT(zeroFee), "non-zero taker fee must exceed the zero-fee case")
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

func (s *RouterTestSuite) newRoutablePool(pool ingesttypes.PoolI, tokenInDenom string, tokenOutDenom string, takerFee osmomath.Dec, cosmWasmConfig domain.CosmWasmPoolRouterConfig) domain.RoutablePool {
	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		Config:                cosmWasmConfig,
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(pool, tokenInDenom, tokenOutDenom, takerFee, cosmWasmPoolsParams)
	s.Require().NoError(err)
	return routablePool
}
