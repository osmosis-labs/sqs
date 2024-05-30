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
				CosmWasmPoolModel: sqsdomain.NewCWPoolModel(
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

			routablePool, err := pools.NewRoutablePool(mock, tc.tokenOut.Denom, noTakerFee, domain.CosmWasmPoolRouterConfig{
				IsAlloyedTransmuterEnabled: true,
			})
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

func (s *RoutablePoolTestSuite) TestFindNormalizationFactors_AlloyTransmuter() {
	tests := map[string]struct {
		tokenInDenom          string
		tokenOutDenom         string
		expectedInNormFactor  osmomath.Int
		expectedOutNormFactor osmomath.Int
		expectError           error
	}{
		"valid normalization factors": {
			tokenInDenom:          USDC,
			tokenOutDenom:         USDT,
			expectedInNormFactor:  osmomath.NewInt(100),
			expectedOutNormFactor: osmomath.NewInt(1),
			expectError:           nil,
		},
		"missing normalization factor for token in": {
			tokenInDenom:          "INVALID",
			tokenOutDenom:         USDT,
			expectedInNormFactor:  osmomath.Int{},
			expectedOutNormFactor: osmomath.NewInt(1),
			expectError:           domain.MissingNormalizationFactorError{Denom: "INVALID", PoolId: 1},
		},
		"missing normalization factor for token out": {
			tokenInDenom:          USDC,
			tokenOutDenom:         "INVALID",
			expectedInNormFactor:  osmomath.NewInt(100),
			expectedOutNormFactor: osmomath.Int{},
			expectError:           domain.MissingNormalizationFactorError{Denom: "INVALID", PoolId: 1},
		},
		"missing normalization factors for both token in and token out": {
			tokenInDenom:          "INVALID1",
			tokenOutDenom:         "INVALID2",
			expectedInNormFactor:  osmomath.Int{},
			expectedOutNormFactor: osmomath.Int{},
			expectError:           domain.MissingNormalizationFactorError{Denom: "INVALID1", PoolId: 1},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{USDC, USDT})

			poolType := cosmwasmPool.GetType()

			mock := &mocks.MockRoutablePool{
				ChainPoolModel: cosmwasmPool.AsSerializablePool(),
				CosmWasmPoolModel: sqsdomain.NewCWPoolModel(
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
				PoolType: poolType,
			}

			routablePool, err := pools.NewRoutablePool(mock, USDT, noTakerFee, domain.CosmWasmPoolRouterConfig{
				IsAlloyedTransmuterEnabled: true,
			})
			s.Require().NoError(err)

			r := routablePool.(*pools.RouteableAlloyTransmuterPoolImpl)

			inNormFactor, outNormFactor, err := r.FindNormalizationFactors(tc.tokenInDenom, tc.tokenOutDenom)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectError)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expectedInNormFactor, inNormFactor)
				s.Require().Equal(tc.expectedOutNormFactor, outNormFactor)
			}
		})
	}
}

func (s *RoutablePoolTestSuite) TestCalcTokenOutAmt_AlloyTransmuter() {
	tests := map[string]struct {
		tokenIn          sdk.Coin
		tokenOutDenom    string
		expectedTokenOut osmomath.BigDec
		expectedError    error
	}{
		"valid calculation using normalization factors": {
			tokenIn:          sdk.NewCoin(USDC, osmomath.NewInt(100)),
			tokenOutDenom:    USDT,
			expectedTokenOut: osmomath.NewBigDec(1), // (100 * 1) / 100 = 1
			expectedError:    nil,
		},
		"valid calculation with decimal points": {
			tokenIn:          sdk.NewCoin(USDC, osmomath.NewInt(10)),
			tokenOutDenom:    USDT,
			expectedTokenOut: osmomath.MustNewBigDecFromStr("0.1"), // (10 * 1) / 100 = 0.1
			expectedError:    nil,
		},
		"valid calculation, truncated to zero": {
			tokenIn:          sdk.NewCoin("overlypreciseusd", osmomath.NewInt(10)),
			tokenOutDenom:    USDC,
			expectedTokenOut: osmomath.MustNewBigDecFromStr("0"),
			expectedError:    nil,
		},
		"missing normalization factor for token in": {
			tokenIn:          sdk.NewCoin("INVALID", osmomath.NewInt(100)),
			tokenOutDenom:    USDT,
			expectedTokenOut: osmomath.ZeroBigDec(),
			expectedError:    domain.MissingNormalizationFactorError{Denom: "INVALID", PoolId: 1},
		},
		"missing normalization factor for token out": {
			tokenIn:          sdk.NewCoin(USDC, osmomath.NewInt(100)),
			tokenOutDenom:    "INVALID",
			expectedTokenOut: osmomath.ZeroBigDec(),
			expectedError:    domain.MissingNormalizationFactorError{Denom: "INVALID", PoolId: 1},
		},
		"missing normalization factors for both token in and token out": {
			tokenIn:          sdk.NewCoin("INVALID", osmomath.NewInt(100)),
			tokenOutDenom:    "INVALID",
			expectedTokenOut: osmomath.ZeroBigDec(),
			expectedError:    domain.MissingNormalizationFactorError{Denom: "INVALID", PoolId: 1},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{USDC, USDT})

			poolType := cosmwasmPool.GetType()

			veryBigNormalizationFactor, _ := osmomath.NewIntFromString("999999999999999999999999999999999999999999")

			mock := &mocks.MockRoutablePool{
				ChainPoolModel: cosmwasmPool.AsSerializablePool(),
				CosmWasmPoolModel: sqsdomain.NewCWPoolModel(
					"crates.io:transmuter", "3.0.0",
					sqsdomain.CWPoolData{
						AlloyTransmuter: &sqsdomain.AlloyTransmuterData{
							AlloyedDenom: ALLUSD,
							AssetConfigs: []sqsdomain.TransmuterAssetConfig{
								{Denom: USDC, NormalizationFactor: osmomath.NewInt(100)},
								{Denom: USDT, NormalizationFactor: osmomath.NewInt(1)},
								{Denom: "overlypreciseusd", NormalizationFactor: veryBigNormalizationFactor},
								{Denom: ALLUSD, NormalizationFactor: osmomath.NewInt(10)},
							},
						},
					},
				),
				PoolType: poolType,
			}

			routablePool, err := pools.NewRoutablePool(mock, USDT, noTakerFee, domain.CosmWasmPoolRouterConfig{
				IsAlloyedTransmuterEnabled: true,
			})
			s.Require().NoError(err)

			r := routablePool.(*pools.RouteableAlloyTransmuterPoolImpl)

			tokenOut, err := r.CalcTokenOutAmt(tc.tokenIn, tc.tokenOutDenom)

			if tc.expectedError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectedError)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expectedTokenOut, tokenOut)
			}
		})
	}
}
