package routertesting

import (
	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

var (
	defaultAmount  = osmomath.NewInt(100_000_00)
	totalInAmount  = defaultAmount
	totalOutAmount = defaultAmount.MulRaw(4)
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

	poolOneLiquidityCap   = osmomath.NewInt(151_9153_195)
	poolTwoLiquidityCap   = osmomath.NewInt(85_196_078)
	poolThreeLiquidityCap = osmomath.NewInt(719_951)
)

func (s *RouterTestHelper) PoolOneLiquidityCap() osmomath.Int {
	return poolOneLiquidityCap
}

func (s *RouterTestHelper) PoolTwoLiquidityCap() osmomath.Int {
	return poolTwoLiquidityCap
}

func (s *RouterTestHelper) PoolThreeLiquidityCap() osmomath.Int {
	return poolThreeLiquidityCap
}

func (s *RouterTestHelper) newRoutablePool(pool ingesttypes.PoolI, tokenInDenom string, tokenOutDenom string, takerFee osmomath.Dec) domain.RoutablePool {
	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(pool, tokenInDenom, tokenOutDenom, takerFee, cosmWasmPoolsParams)

	s.Require().NoError(err)

	return routablePool
}

func (s *RouterTestHelper) NewExactAmountInQuote(p1, p2, p3 poolmanagertypes.PoolI) *usecase.QuoteExactAmountIn {
	return &usecase.QuoteExactAmountIn{
		AmountIn:  sdk.NewCoin(ETH, totalInAmount),
		AmountOut: totalOutAmount,

		// 2 routes with 50-50 split, each single hop
		Route: []domain.SplitRoute{
			// Route 1
			&route.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							ingesttypes.NewPool(p1, ingesttypes.SQSPool{
								SpreadFactor:     p1.GetSpreadFactor(sdk.Context{}),
								Balances:         poolOneBalances,
								PoolLiquidityCap: poolOneLiquidityCap,
							}),
							ETH,
							USDT,
							takerFeeOne,
						),
						s.newRoutablePool(
							ingesttypes.NewPool(p2, ingesttypes.SQSPool{
								SpreadFactor:     p2.GetSpreadFactor(sdk.Context{}),
								Balances:         poolTwoBalances,
								PoolLiquidityCap: poolTwoLiquidityCap,
							}),
							USDT,
							USDC,
							takerFeeTwo,
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
						s.newRoutablePool(
							ingesttypes.NewPool(p3, ingesttypes.SQSPool{
								SpreadFactor:     p3.GetSpreadFactor(sdk.Context{}),
								Balances:         poolThreeBalances,
								PoolLiquidityCap: poolThreeLiquidityCap,
							}),
							ETH,
							USDC,
							takerFeeThree,
						),
					},
				},

				InAmount:  totalInAmount.QuoRaw(2),
				OutAmount: totalOutAmount.QuoRaw(2),
			},
		},
		EffectiveFee: osmomath.ZeroDec(),
	}
}

// NewExactAmountOutTrueQuote creates an exact-amount-out quote that exercises the
// "true" in-given-out PrepareResult path (the embedded quoteExactAmountIn is left nil),
// as produced by estimateAndRankSingleRouteQuoteInGivenOut after the exact-out wiring.
//
// This differs from NewExactAmountOutQuote, which sets the embedded quoteExactAmountIn
// and therefore exercises the legacy inversion fallback branch of PrepareResult.
//
// The quote is a 2-route split:
//   - Route 1: 2 hops (pool1: in->USDT, pool2: USDT->out), receives 3/5 of the output.
//   - Route 2: 1 hop  (pool3: in->out),                    receives 2/5 of the output.
//
// AmountIn/AmountOut and the per-route In/OutAmounts are caller-supplied so a test can
// assert exact effective-fee and price-impact arithmetic. The per-route OutAmounts must
// sum to amountOut and the per-route InAmounts must sum to amountIn, since PrepareResult
// weights each route by route.OutAmount / totalAmountOut.
func (s *RouterTestHelper) NewExactAmountOutTrueQuote(
	p1, p2, p3 poolmanagertypes.PoolI,
	amountIn osmomath.Int,
	amountOut sdk.Coin,
	routeOneIn, routeOneOut osmomath.Int,
	routeTwoIn, routeTwoOut osmomath.Int,
) *usecase.QuoteExactAmountOut {
	return &usecase.QuoteExactAmountOut{
		// quoteExactAmountIn intentionally left nil to exercise the true exact-out path.
		AmountIn:  amountIn,
		AmountOut: amountOut,
		Route: []domain.SplitRoute{
			// Route 1: 2 hops, in -> USDT -> out
			&route.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							ingesttypes.NewPool(p1, ingesttypes.SQSPool{
								SpreadFactor:     p1.GetSpreadFactor(sdk.Context{}),
								Balances:         poolOneBalances,
								PoolLiquidityCap: poolOneLiquidityCap,
							}),
							ETH,
							USDT,
							takerFeeOne,
						),
						s.newRoutablePool(
							ingesttypes.NewPool(p2, ingesttypes.SQSPool{
								SpreadFactor:     p2.GetSpreadFactor(sdk.Context{}),
								Balances:         poolTwoBalances,
								PoolLiquidityCap: poolTwoLiquidityCap,
							}),
							USDT,
							USDC,
							takerFeeTwo,
						),
					},
				},
				InAmount:  routeOneIn,
				OutAmount: routeOneOut,
			},
			// Route 2: 1 hop, in -> out
			&route.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							ingesttypes.NewPool(p3, ingesttypes.SQSPool{
								SpreadFactor:     p3.GetSpreadFactor(sdk.Context{}),
								Balances:         poolThreeBalances,
								PoolLiquidityCap: poolThreeLiquidityCap,
							}),
							ETH,
							USDC,
							takerFeeThree,
						),
					},
				},
				InAmount:  routeTwoIn,
				OutAmount: routeTwoOut,
			},
		},
		EffectiveFee: osmomath.ZeroDec(),
	}
}

// NewExactAmountOutQuote creates a new exact amount out.
// NOTE: It is not possible to access the usecase.QuoteImpl struct directly from here.
func (s *RouterTestHelper) NewExactAmountOutQuote(p1, p2, p3 poolmanagertypes.PoolI) *usecase.QuoteExactAmountOut {
	return usecase.NewQuoteExactAmountOut(&usecase.QuoteExactAmountIn{
		AmountIn:  sdk.NewCoin(ETH, totalInAmount),
		AmountOut: totalOutAmount,

		// 2 routes with 50-50 split, each single hop
		Route: []domain.SplitRoute{
			&route.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							ingesttypes.NewPool(p1, ingesttypes.SQSPool{
								SpreadFactor:     p1.GetSpreadFactor(sdk.Context{}),
								Balances:         poolOneBalances,
								PoolLiquidityCap: poolOneLiquidityCap,
							}),
							ETH,
							USDT,
							takerFeeOne,
						),
						s.newRoutablePool(
							ingesttypes.NewPool(p2, ingesttypes.SQSPool{
								SpreadFactor:     p2.GetSpreadFactor(sdk.Context{}),
								Balances:         poolTwoBalances,
								PoolLiquidityCap: poolTwoLiquidityCap,
							}),
							USDT,
							USDC,
							takerFeeTwo,
						),
					},
				},

				InAmount:  totalInAmount.QuoRaw(2),
				OutAmount: totalOutAmount.QuoRaw(3),
			},
			&route.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							ingesttypes.NewPool(p3, ingesttypes.SQSPool{
								SpreadFactor:     p3.GetSpreadFactor(sdk.Context{}),
								Balances:         poolThreeBalances,
								PoolLiquidityCap: poolThreeLiquidityCap,
							}),
							ETH,
							USDC,
							takerFeeThree,
						),
					},
				},

				InAmount:  totalInAmount.QuoRaw(4),
				OutAmount: totalOutAmount.QuoRaw(5),
			},
		},
		EffectiveFee: osmomath.ZeroDec(),
	})
}
