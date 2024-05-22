package pools_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// Tests no slippage quotes and validation edge cases aroun transmuter pools.
func (s *RoutablePoolTestSuite) TestCalculateTokenOutByTokenIn_AlloyTransmuter() {
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, osmomath.NewInt(1000000)), sdk.NewCoin(USDT, osmomath.NewInt(1000000)))

	tests := map[string]struct {
		tokenIn     sdk.Coin
		tokenOut    sdk.Coin
		balances    sdk.Coins
		expectError error
	}{
		"valid transmuter quote": {
			tokenIn:  sdk.NewCoin(USDT, osmomath.NewInt(10000)),
			tokenOut: sdk.NewCoin(USDC, osmomath.NewInt(1000000)),
			balances: defaultBalances,
		},
		"no error: token in is larger than balance of token in": {
			tokenIn:  sdk.NewCoin(USDC, osmomath.NewInt(1000001)),
			tokenOut: sdk.NewCoin(USDT, osmomath.NewInt(10000)),
			balances: defaultBalances,
		},
		"no error: token out is larger thgan balance of token out but token out is an alloyed": {
			tokenIn:  sdk.NewCoin(USDT, osmomath.NewInt(1000001)),
			tokenOut: sdk.NewCoin(ALLUSD, osmomath.NewInt(10000010)),
			balances: defaultBalances,
		},
		"error: token out is larger than balance of token out": {
			tokenIn:  sdk.NewCoin(USDT, osmomath.NewInt(10001)),
			tokenOut: sdk.NewCoin(USDC, osmomath.NewInt(1000100)),
			balances: defaultBalances,
			expectError: domain.TransmuterInsufficientBalanceError{
				Denom:         USDC,
				BalanceAmount: osmomath.NewInt(1000000).String(),
				Amount:        osmomath.NewInt(1000100).String(),
			},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{tc.tokenIn.Denom, tc.tokenOut.Denom})

			poolType := cosmwasmPool.GetType()

			mock := &mocks.MockRoutablePool{
				ChainPoolModel: cosmwasmPool.AsSerializablePool(),
				CWPoolModel: sqsdomain.NewCWPoolModel(
					"crates.io:transmuter", "3.0.0",
					sqsdomain.CWPoolData{
						AlloyTransmuter: &sqsdomain.AlloyTransmuterData{
							AlloyedDenom: ALLUSD,
							AssetConfigs: []sqsdomain.TransmuterAssetConfig{
								{Denom: USDC, NormalizationFactor: osmomath.NewInt(100)},
								{Denom: USDT, NormalizationFactor: osmomath.NewInt(1)},
								{Denom: ALLUSD, NormalizationFactor: osmomath.NewInt(10)},
							},
						},
					},
				),
				Balances: tc.balances,
				PoolType: poolType,
			}

			routablePool, err := pools.NewRoutablePool(mock, tc.tokenOut.Denom, noTakerFee, domain.CosmWasmPoolRouterConfig{})
			s.Require().NoError(err)

			tokenOut, err := routablePool.CalculateTokenOutByTokenIn(context.TODO(), tc.tokenIn)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectError)
				return
			}
			s.Require().NoError(err)
			s.Require().Equal(tc.tokenOut, tokenOut)
		})
	}
}
