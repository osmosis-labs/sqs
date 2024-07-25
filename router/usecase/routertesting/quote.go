package routertesting

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

var (
	DefaultAmount  = sdk.NewInt(100_000_00)
	TotalInAmount  = DefaultAmount
	TotalOutAmount = DefaultAmount.MulRaw(4)
	TokenIn        = sdk.NewCoin(ETH, TotalInAmount)
)

var (
	takerFeeOne   = osmomath.NewDecWithPrec(2, 2)
	takerFeeTwo   = osmomath.NewDecWithPrec(4, 4)
	takerFeeThree = osmomath.NewDecWithPrec(3, 3)

	poolOneBalances = sdk.NewCoins(
		sdk.NewCoin(USDT, DefaultAmount.MulRaw(5)),
		sdk.NewCoin(ETH, DefaultAmount),
	)

	poolTwoBalances = sdk.NewCoins(
		sdk.NewCoin(USDC, DefaultAmount),
		sdk.NewCoin(USDT, DefaultAmount),
	)

	poolThreeBalances = sdk.NewCoins(
		sdk.NewCoin(ETH, DefaultAmount),
		sdk.NewCoin(USDC, DefaultAmount.MulRaw(4)),
	)
)

func (s *RouterTestHelper) newRoutablePool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec, cosmWasmConfig domain.CosmWasmPoolRouterConfig) domain.RoutablePool {
	cosmWasmPoolsParams := pools.CosmWasmPoolsParams{
		Config:                cosmWasmConfig,
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(pool, tokenOutDenom, takerFee, cosmWasmPoolsParams)

	s.Require().NoError(err)

	return routablePool
}

// NewExactAmountInQuote creates a new exact amount in Quote.
// AmountIn is filled with s.NewAmountIn() and AmountOut is filled with s.NewAmountOut().
func (s *RouterTestHelper) NewExactAmountInQuote(p1, p2, p3 poolmanagertypes.PoolI) *usecase.QuoteExactAmountIn {
	return &usecase.QuoteExactAmountIn{
		AmountIn:  TokenIn,
		AmountOut: TotalOutAmount,

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
							domain.CosmWasmPoolRouterConfig{},
						),
						s.newRoutablePool(
							sqsdomain.NewPool(p2, p2.GetSpreadFactor(sdk.Context{}), poolTwoBalances),
							USDC,
							takerFeeTwo,
							domain.CosmWasmPoolRouterConfig{},
						),
					},
				},

				InAmount:  TotalInAmount.QuoRaw(2),
				OutAmount: TotalOutAmount.QuoRaw(2),
			},

			// Route 2
			&usecase.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							sqsdomain.NewPool(p3, p3.GetSpreadFactor(sdk.Context{}), poolThreeBalances),
							USDC,
							takerFeeThree,
							domain.CosmWasmPoolRouterConfig{},
						),
					},
				},

				InAmount:  TotalInAmount.QuoRaw(2),
				OutAmount: TotalOutAmount.QuoRaw(2),
			},
		},
		EffectiveFee: osmomath.ZeroDec(),
	}
}

// NewExactAmountOutQuote creates a new exact amount out.
// NOTE: It is not possible to access the usecase.QuoteImpl struct directly from here.
func (s *RouterTestHelper) NewExactAmountOutQuote(p1, p2, p3 poolmanagertypes.PoolI) *usecase.QuoteExactAmountOut {
	return usecase.NewQuoteExactAmountOut(&usecase.QuoteExactAmountIn{
		AmountIn:  TokenIn,
		AmountOut: TotalOutAmount,

		// 2 routes with 50-50 split, each single hop
		Route: []domain.SplitRoute{
			&usecase.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							sqsdomain.NewPool(p1, p1.GetSpreadFactor(sdk.Context{}), poolOneBalances),
							USDT,
							takerFeeOne,
							domain.CosmWasmPoolRouterConfig{},
						),
						s.newRoutablePool(
							sqsdomain.NewPool(p2, p2.GetSpreadFactor(sdk.Context{}), poolTwoBalances),
							USDC,
							takerFeeTwo,
							domain.CosmWasmPoolRouterConfig{},
						),
					},
				},

				InAmount:  TotalInAmount.QuoRaw(2),
				OutAmount: TotalOutAmount.QuoRaw(3),
			},
			&usecase.RouteWithOutAmount{
				RouteImpl: route.RouteImpl{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(
							sqsdomain.NewPool(p3, p3.GetSpreadFactor(sdk.Context{}), poolThreeBalances),
							USDC,
							takerFeeThree,
							domain.CosmWasmPoolRouterConfig{},
						),
					},
				},

				InAmount:  TotalInAmount.QuoRaw(4),
				OutAmount: TotalOutAmount.QuoRaw(5),
			},
		},
		EffectiveFee: osmomath.ZeroDec(),
	})
}
