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

const (
	OVERLY_PRECISE_USD = "overlypreciseusd"
	NO_PRECISION_USD   = "noprecisionusd"
	INVALID_DENOM      = "invalid_denom"
	MORE_INVALID_DENOM = "more_invalid_denom"
)

func (s *RoutablePoolTestSuite) SetupRoutableAlloyTransmuterPool(tokenInDenom, tokenOutDenom string, balances sdk.Coins) sqsdomain.RoutablePool {
	cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{tokenInDenom, tokenOutDenom})

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
						{Denom: OVERLY_PRECISE_USD, NormalizationFactor: veryBigNormalizationFactor},
						{Denom: NO_PRECISION_USD, NormalizationFactor: osmomath.ZeroInt()},
						{Denom: ALLUSD, NormalizationFactor: osmomath.NewInt(10)},
					},
				},
			},
		),
		Balances: balances,
		PoolType: poolType,
	}

	routablePool, err := pools.NewRoutablePool(mock, tokenOutDenom, noTakerFee, domain.CosmWasmPoolRouterConfig{
		AlloyedTransmuterCodeIDs: map[uint64]struct{}{
			defaultPoolID: {},
		},
	}, domain.UnsetScalingFactorGetterCb)
	s.Require().NoError(err)

	return routablePool
}

// Tests no slippage quotes and validation edge cases aroun transmuter pools.
func (s *RoutablePoolTestSuite) TestCalculateTokenOutByTokenIn_AlloyTransmuter() {
	defaltBalanceAmt := osmomath.NewInt(1000000)
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, defaltBalanceAmt), sdk.NewCoin(USDT, defaltBalanceAmt))

	tests := map[string]struct {
		tokenIn     sdk.Coin
		tokenOut    sdk.Coin
		balances    sdk.Coins
		expectError error
	}{
		"valid transmuter quote": {
			tokenIn:  sdk.NewCoin(USDT, osmomath.NewInt(10000)),
			tokenOut: sdk.NewCoin(USDC, defaltBalanceAmt),
			balances: defaultBalances,
		},
		"trancate to 0": {
			tokenIn:  sdk.NewCoin(OVERLY_PRECISE_USD, osmomath.NewInt(10)),
			tokenOut: sdk.NewCoin(USDC, osmomath.NewInt(0)),
			balances: defaultBalances,
		},
		"no error: token in is larger than balance of token in": {
			tokenIn:  sdk.NewCoin(USDC, defaltBalanceAmt.Add(osmomath.NewInt(1))),
			tokenOut: sdk.NewCoin(USDT, osmomath.NewInt(10000)),
			balances: defaultBalances,
		},
		"no error: token out is larger than balance of token out but token out is an alloyed": {
			tokenIn:  sdk.NewCoin(USDT, defaltBalanceAmt.Add(osmomath.NewInt(1))),
			tokenOut: sdk.NewCoin(ALLUSD, defaltBalanceAmt.Add(osmomath.NewInt(1)).Mul(osmomath.NewInt(10))),
			balances: defaultBalances,
		},
		"error: zero token in normalization factor": {
			tokenIn:  sdk.NewCoin(NO_PRECISION_USD, osmomath.NewInt(10000)),
			tokenOut: sdk.NewCoin(ALLUSD, osmomath.NewInt(0)),
			balances: defaultBalances,
			expectError: domain.ZeroNormalizationFactorError{
				Denom:  NO_PRECISION_USD,
				PoolId: defaultPoolID,
			},
		},
		"error: zero token out normalization factor": {
			tokenIn:  sdk.NewCoin(ALLUSD, osmomath.NewInt(10000)),
			tokenOut: sdk.NewCoin(NO_PRECISION_USD, osmomath.NewInt(0)),
			balances: defaultBalances,
			expectError: domain.ZeroNormalizationFactorError{
				Denom:  NO_PRECISION_USD,
				PoolId: defaultPoolID,
			},
		},
		"error: token out is larger than balance of token out": {
			tokenIn:  sdk.NewCoin(USDT, osmomath.NewInt(10001)),
			tokenOut: sdk.NewCoin(USDC, defaltBalanceAmt.Add(osmomath.NewInt(100))),
			balances: defaultBalances,
			expectError: domain.TransmuterInsufficientBalanceError{
				Denom:         USDC,
				BalanceAmount: defaltBalanceAmt.String(),
				Amount:        defaltBalanceAmt.Add(osmomath.NewInt(100)).String(),
			},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableAlloyTransmuterPool(tc.tokenIn.Denom, tc.tokenOut.Denom, tc.balances)
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
			tokenInDenom:          INVALID_DENOM,
			tokenOutDenom:         USDT,
			expectedInNormFactor:  osmomath.Int{},
			expectedOutNormFactor: osmomath.NewInt(1),
			expectError:           domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
		"missing normalization factor for token out": {
			tokenInDenom:          USDC,
			tokenOutDenom:         INVALID_DENOM,
			expectedInNormFactor:  osmomath.NewInt(100),
			expectedOutNormFactor: osmomath.Int{},
			expectError:           domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
		"missing normalization factors for both token in and token out": {
			tokenInDenom:          INVALID_DENOM,
			tokenOutDenom:         MORE_INVALID_DENOM,
			expectedInNormFactor:  osmomath.Int{},
			expectedOutNormFactor: osmomath.Int{},
			expectError:           domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableAlloyTransmuterPool(tc.tokenInDenom, tc.tokenOutDenom, sdk.Coins{})

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
			tokenIn:          sdk.NewCoin(OVERLY_PRECISE_USD, osmomath.NewInt(10)),
			tokenOutDenom:    USDC,
			expectedTokenOut: osmomath.MustNewBigDecFromStr("0"),
			expectedError:    nil,
		},
		"missing normalization factor for token in": {
			tokenIn:          sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(100)),
			tokenOutDenom:    USDT,
			expectedTokenOut: osmomath.BigDec{},
			expectedError:    domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
		"missing normalization factor for token out": {
			tokenIn:          sdk.NewCoin(USDC, osmomath.NewInt(100)),
			tokenOutDenom:    INVALID_DENOM,
			expectedTokenOut: osmomath.BigDec{},
			expectedError:    domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
		"missing normalization factors for both token in and token out": {
			tokenIn:          sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(100)),
			tokenOutDenom:    INVALID_DENOM,
			expectedTokenOut: osmomath.BigDec{},
			expectedError:    domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			routablePool := s.SetupRoutableAlloyTransmuterPool(tc.tokenIn.Denom, tc.tokenOutDenom, sdk.Coins{})

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
