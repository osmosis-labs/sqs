package routertesting

import (
	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/sqsdomain"

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
)

func (s *RouterTestHelper) newRoutablePool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec) domain.RoutablePool {
	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(pool, tokenOutDenom, takerFee, cosmWasmPoolsParams)

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
			&usecase.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							sqsdomain.NewPool(p1, p1.GetSpreadFactor(sdk.Context{}), poolOneBalances),
							USDT,
							takerFeeOne,
						),
						s.newRoutablePool(
							sqsdomain.NewPool(p2, p2.GetSpreadFactor(sdk.Context{}), poolTwoBalances),
							USDC,
							takerFeeTwo,
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
							sqsdomain.NewPool(p3, p3.GetSpreadFactor(sdk.Context{}), poolThreeBalances),
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

// NewExactAmountOutQuote creates a new exact amount out.
// NOTE: It is not possible to access the usecase.QuoteImpl struct directly from here.
func (s *RouterTestHelper) NewExactAmountOutQuote(p1, p2, p3 poolmanagertypes.PoolI) *usecase.QuoteExactAmountOut {
	return usecase.NewQuoteExactAmountOut(&usecase.QuoteExactAmountIn{
		AmountIn:  sdk.NewCoin(ETH, totalInAmount),
		AmountOut: totalOutAmount,

		// 2 routes with 50-50 split, each single hop
		Route: []domain.SplitRoute{
			&usecase.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							sqsdomain.NewPool(p1, p1.GetSpreadFactor(sdk.Context{}), poolOneBalances),
							USDT,
							takerFeeOne,
						),
						s.newRoutablePool(
							sqsdomain.NewPool(p2, p2.GetSpreadFactor(sdk.Context{}), poolTwoBalances),
							USDC,
							takerFeeTwo,
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
							sqsdomain.NewPool(p3, p3.GetSpreadFactor(sdk.Context{}), poolThreeBalances),
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
