package pools_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/v30/ingest/types/cosmwasmpool"
	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/osmomath"
)

const (
	OVERLY_PRECISE_USD = "overlypreciseusd"
	NO_PRECISION_USD   = "noprecisionusd"
	INVALID_DENOM      = "invalid_denom"
	MORE_INVALID_DENOM = "more_invalid_denom"
)

func (s *RoutablePoolTestSuite) SetupRoutableAlloyTransmuterPool(tokenInDenom, tokenOutDenom string, balances sdk.Coins, takerFee osmomath.Dec) domain.RoutablePool {
	// Provide default normalization scaling factors and balances for tests.
	if len(balances) == 0 {
		balances = sdk.NewCoins(
			sdk.NewCoin(USDC, osmomath.NewInt(1_000_000)),
			sdk.NewCoin(USDT, osmomath.NewInt(1_000_000)),
		)
	}

	defaultPrecomputed := cosmwasmpool.PrecomputedData{
		StdNormFactor: osmomath.NewInt(1),
		NormalizationScalingFactors: map[string]osmomath.Int{
			USDC:               osmomath.NewInt(1),
			USDT:               osmomath.NewInt(1),
			OVERLY_PRECISE_USD: osmomath.NewInt(1),
			NO_PRECISION_USD:   osmomath.NewInt(1),
			ALLUSD:             osmomath.NewInt(1),
		},
	}

	return s.SetupRoutableAlloyTransmuterPoolCustom(tokenInDenom, tokenOutDenom, balances, takerFee, cosmwasmpool.RebalancingConfigs{}, defaultPrecomputed)
}

func (s *RoutablePoolTestSuite) SetupRoutableAlloyTransmuterPoolCustom(tokenInDenom, tokenOutDenom string, balances sdk.Coins, takerFee osmomath.Dec, rebalancingConfigs cosmwasmpool.RebalancingConfigs, preComputedData cosmwasmpool.PrecomputedData) domain.RoutablePool {
	cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{tokenInDenom, tokenOutDenom})

	poolType := cosmwasmPool.GetType()

	veryBigNormalizationFactor, _ := osmomath.NewIntFromString("999999999999999999999999999999999999999999")

	mock := &mocks.MockRoutablePool{
		ChainPoolModel: cosmwasmPool.AsSerializablePool(),
		CosmWasmPoolModel: cosmwasmpool.NewCWPoolModel(
			cosmwasmpool.ALLOY_TRANSMUTER_CONTRACT_NAME, cosmwasmpool.ALLOY_TRANSMUTER_MIN_CONTRACT_VERSION,
			cosmwasmpool.CosmWasmPoolData{
				AlloyTransmuter: &cosmwasmpool.AlloyTransmuterData{
					AlloyedDenom: ALLUSD,
					AssetConfigs: []cosmwasmpool.TransmuterAssetConfig{
						{Denom: OVERLY_PRECISE_USD, NormalizationFactor: veryBigNormalizationFactor},
						{Denom: NO_PRECISION_USD, NormalizationFactor: osmomath.ZeroInt()},
						{Denom: USDT, NormalizationFactor: osmomath.NewInt(1)},
						{Denom: USDC, NormalizationFactor: osmomath.NewInt(100)},
						{Denom: ALLUSD, NormalizationFactor: osmomath.NewInt(10)},
					},

					RebalancingConfigs: rebalancingConfigs,
					PreComputedData:    preComputedData,
				},
			},
		),
		Balances: balances,
		PoolType: poolType,
		TakerFee: takerFee,
	}

	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		Config: domain.CosmWasmPoolRouterConfig{
			AlloyedTransmuterCodeIDs: map[uint64]struct{}{
				defaultPoolID: {},
			},
		},
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(mock, tokenInDenom, tokenOutDenom, takerFee, cosmWasmPoolsParams)
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
			routablePool := s.SetupRoutableAlloyTransmuterPool(tc.tokenIn.Denom, tc.tokenOut.Denom, tc.balances, osmomath.ZeroDec())
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
			routablePool := s.SetupRoutableAlloyTransmuterPool(tc.tokenInDenom, tc.tokenOutDenom, sdk.Coins{}, osmomath.ZeroDec())

			r := routablePool.(*pools.RoutableAlloyTransmuterPoolImpl)

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

			routablePool := s.SetupRoutableAlloyTransmuterPool(tc.tokenIn.Denom, tc.tokenOutDenom, sdk.Coins{}, osmomath.ZeroDec())

			r := routablePool.(*pools.RoutableAlloyTransmuterPoolImpl)

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

func (s *RoutablePoolTestSuite) TestCalcTokenInAmt_AlloyTransmuter() {
	tests := map[string]struct {
		tokenOut         sdk.Coin
		tokenInDenom     string
		expectedTokenOut osmomath.BigDec
		expectedError    error
	}{
		"valid calculation using normalization factors": {
			tokenOut:         sdk.NewCoin(USDC, osmomath.NewInt(100)),
			tokenInDenom:     USDT,
			expectedTokenOut: osmomath.NewBigDec(1), // (100 * 1) / 100 = 1
			expectedError:    nil,
		},
		"valid calculation with decimal points": {
			tokenOut:         sdk.NewCoin(USDC, osmomath.NewInt(10)),
			tokenInDenom:     USDT,
			expectedTokenOut: osmomath.MustNewBigDecFromStr("0.1"), // (10 * 1) / 100 = 0.1
			expectedError:    nil,
		},
		"valid calculation, truncated to zero": {
			tokenOut:         sdk.NewCoin(OVERLY_PRECISE_USD, osmomath.NewInt(10)),
			tokenInDenom:     USDC,
			expectedTokenOut: osmomath.MustNewBigDecFromStr("0"),
			expectedError:    nil,
		},
		"missing normalization factor for token in": {
			tokenOut:         sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(100)),
			tokenInDenom:     USDT,
			expectedTokenOut: osmomath.BigDec{},
			expectedError:    domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
		"missing normalization factor for token out": {
			tokenOut:         sdk.NewCoin(USDC, osmomath.NewInt(100)),
			tokenInDenom:     INVALID_DENOM,
			expectedTokenOut: osmomath.BigDec{},
			expectedError:    domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
		"missing normalization factors for both token in and token out": {
			tokenOut:         sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(100)),
			tokenInDenom:     INVALID_DENOM,
			expectedTokenOut: osmomath.BigDec{},
			expectedError:    domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			routablePool := s.SetupRoutableAlloyTransmuterPool(tc.tokenInDenom, tc.tokenOut.Denom, sdk.Coins{}, osmomath.ZeroDec())

			r := routablePool.(*pools.RoutableAlloyTransmuterPoolImpl)

			tokenIn, err := r.CalcTokenInAmt(tc.tokenOut, tc.tokenInDenom)

			if tc.expectedError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectedError)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expectedTokenOut, tokenIn)
			}
		})
	}
}

func (s *RoutablePoolTestSuite) TestChargeTakerFeeExactIn_AlloyTransmuter() {
	tests := map[string]struct {
		tokenIn       sdk.Coin
		takerFee      osmomath.Dec
		expectedToken sdk.Coin
	}{
		"no taker fee": {
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDec(0),
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(100)),
		},
		"small taker fee": {
			tokenIn:       sdk.NewCoin(USDT, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(1, 2),          // 1%
			expectedToken: sdk.NewCoin(USDT, osmomath.NewInt(99)), // 100 - 1 = 99
		},
		"large taker fee": {
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(5, 1),          // 50%
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(50)), // 100 - 50 = 50
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableAlloyTransmuterPool(tc.tokenIn.Denom, tc.tokenIn.Denom, sdk.Coins{}, tc.takerFee)

			r := routablePool.(*pools.RoutableAlloyTransmuterPoolImpl)

			tokenAfterFee := r.ChargeTakerFeeExactIn(tc.tokenIn)

			s.Require().Equal(tc.expectedToken, tokenAfterFee)
		})
	}
}

func (s *RoutablePoolTestSuite) TestCheckStaticRateLimiter() {
	// Shared default variables
	defaultScalingFactors := map[string]osmomath.Int{
		USDC:               osmomath.NewInt(1),
		USDT:               osmomath.NewInt(1),
		OVERLY_PRECISE_USD: osmomath.NewInt(1),
		NO_PRECISION_USD:   osmomath.NewInt(1),
		ALLUSD:             osmomath.NewInt(1),
	}

	oneInt := osmomath.NewInt(1)

	defaultStandardNormFactor := osmomath.NewInt(1)

	defaultInitialBalances := sdk.NewCoins(
		sdk.NewCoin(USDC, osmomath.NewInt(1_000_000)),
		sdk.NewCoin(USDT, osmomath.NewInt(2_000_000)),
	)

	defaultRebalancingConfigs := map[string]cosmwasmpool.RebalancingConfig{
		pools.DenomPrefix + USDC: {Limit: "0.5"},
	}

	tests := map[string]struct {
		tokenInCoin                 sdk.Coin
		tokenOutDenom               string
		initialBalances             sdk.Coins
		standardNormFactor          osmomath.Int
		normalizationScalingFactors map[string]osmomath.Int
		rebalancingConfigs          map[string]cosmwasmpool.RebalancingConfig
		assetGroups                 map[string]cosmwasmpool.AssetGroup
		expectError                 error
	}{
		"valid token in - below upper limit": {
			tokenInCoin:                 sdk.NewCoin(USDC, osmomath.NewInt(100_000)),
			tokenOutDenom:               USDT,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			rebalancingConfigs:          defaultRebalancingConfigs,
			expectError:                 nil,
		},
		"invalid token in - exceeds upper limit": {
			tokenInCoin:                 sdk.NewCoin(USDC, osmomath.NewInt(2_000_000)),
			tokenOutDenom:               USDT,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			rebalancingConfigs:          defaultRebalancingConfigs,
			expectError: domain.StaticRateLimiterInvalidUpperLimitError{
				Scope:      pools.DenomPrefix + USDC,
				UpperLimit: "0.5",
				Weight:     osmomath.MustNewDecFromStr("1").String(),
			},
		},
		"no static limiter configured": {
			tokenInCoin:                 sdk.NewCoin(USDC, osmomath.NewInt(1_000_000)),
			tokenOutDenom:               USDT,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			rebalancingConfigs:          map[string]cosmwasmpool.RebalancingConfig{},
			expectError:                 nil,
		},
		"static limiter not set for token in denom": {
			tokenInCoin:                 sdk.NewCoin(USDT, osmomath.NewInt(1_000_000)),
			tokenOutDenom:               USDC,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			rebalancingConfigs:          defaultRebalancingConfigs,
			expectError:                 nil,
		},
		"check all assets' static limiters when token in denom is alloyed": {
			tokenInCoin:                 sdk.NewCoin(ALLUSD, osmomath.NewInt(1_000_001)),
			tokenOutDenom:               USDT,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			rebalancingConfigs:          defaultRebalancingConfigs,
			expectError: domain.StaticRateLimiterInvalidUpperLimitError{
				Scope:      pools.DenomPrefix + USDC,
				UpperLimit: "0.5",
				Weight:     osmomath.MustNewDecFromStr("0.500000250000125").String(),
			},
		},
		"different normalization factors": {
			tokenInCoin:   sdk.NewCoin(USDC, osmomath.NewInt(500_000)),
			tokenOutDenom: USDT,
			initialBalances: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(1_000_000)), // + 500_000 = 1_500_000
				sdk.NewCoin(USDT, osmomath.NewInt(2_000_000)), // - 1_000_000 = 1_000_000
			),
			standardNormFactor: defaultStandardNormFactor,
			normalizationScalingFactors: map[string]osmomath.Int{
				USDC:               osmomath.NewInt(2),
				USDT:               oneInt,
				OVERLY_PRECISE_USD: oneInt,
				NO_PRECISION_USD:   oneInt,
			},
			rebalancingConfigs: map[string]cosmwasmpool.RebalancingConfig{
				pools.DenomPrefix + USDC: {Limit: "0.7"},
			},
			expectError: domain.StaticRateLimiterInvalidUpperLimitError{
				Scope:      pools.DenomPrefix + USDC,
				UpperLimit: "0.7",
				Weight:     osmomath.MustNewDecFromStr("0.750000000000000000").String(),
			},
		},
		"asset group limit exceeded (sum over limit)": {
			tokenInCoin:   sdk.NewCoin(USDC, osmomath.NewInt(100_000)),
			tokenOutDenom: USDT,
			initialBalances: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(1_000_000)),
				sdk.NewCoin(USDT, osmomath.NewInt(1_000_000)),
				sdk.NewCoin(OVERLY_PRECISE_USD, osmomath.NewInt(1_000_000)),
			),
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			rebalancingConfigs: map[string]cosmwasmpool.RebalancingConfig{
				"asset_group::STABLE": {Limit: "0.68"},
			},
			assetGroups: map[string]cosmwasmpool.AssetGroup{
				"STABLE": {Denoms: []string{USDC, OVERLY_PRECISE_USD}},
			},
			expectError: domain.StaticRateLimiterInvalidUpperLimitError{
				Scope:      "asset_group::STABLE",
				UpperLimit: "0.68",
				Weight:     osmomath.MustNewDecFromStr("0.700000000000000000").String(),
			},
		},
		"asset group limit respected (under limit)": {
			tokenInCoin:   sdk.NewCoin(USDC, osmomath.NewInt(100_000)),
			tokenOutDenom: USDT,
			initialBalances: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(1_000_000)),
				sdk.NewCoin(USDT, osmomath.NewInt(1_000_000)),
				sdk.NewCoin(OVERLY_PRECISE_USD, osmomath.NewInt(1_000_000)),
			),
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			rebalancingConfigs: map[string]cosmwasmpool.RebalancingConfig{
				"asset_group::STABLE": {Limit: "0.71"},
			},
			assetGroups: map[string]cosmwasmpool.AssetGroup{
				"STABLE": {Denoms: []string{USDC, OVERLY_PRECISE_USD}},
			},
			expectError: nil,
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableAlloyTransmuterPoolCustom(tc.tokenInCoin.Denom, tc.tokenOutDenom, tc.initialBalances, osmomath.ZeroDec(),
				tc.rebalancingConfigs,
				cosmwasmpool.PrecomputedData{
					StdNormFactor:               tc.standardNormFactor,
					NormalizationScalingFactors: tc.normalizationScalingFactors,
				},
			)

			r := routablePool.(*pools.RoutableAlloyTransmuterPoolImpl)
			// Inject asset groups if provided by the test case
			if tc.assetGroups != nil {
				r.AlloyTransmuterData.AssetGroups = tc.assetGroups
			}

			tokenOutCoin := sdk.NewCoin(tc.tokenOutDenom, tc.tokenInCoin.Amount.Mul(tc.normalizationScalingFactors[tc.tokenInCoin.Denom]).Quo(tc.normalizationScalingFactors[tc.tokenOutDenom]))

			// System under test
			err := r.CheckStaticRateLimiter(tc.tokenInCoin, tokenOutCoin)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectError)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

func (s *RoutablePoolTestSuite) TestComputeTotalAdjustmentRate() {
	// Helper function to create rebalancing config
	createRebalancingConfig := func(idealUpper, idealLower, criticalUpper, criticalLower, limit, adjustmentRateStrained, adjustmentRateCritical string) cosmwasmpool.RebalancingConfig {
		return cosmwasmpool.RebalancingConfig{
			IdealUpper:             idealUpper,
			IdealLower:             idealLower,
			CriticalUpper:          criticalUpper,
			CriticalLower:          criticalLower,
			Limit:                  limit,
			AdjustmentRateStrained: adjustmentRateStrained,
			AdjustmentRateCritical: adjustmentRateCritical,
		}
	}

	testRebalancingConfig := createRebalancingConfig(
		"0.70", // ideal_upper (70%)
		"0.30", // ideal_lower (30%)
		"0.80", // critical_upper (80%)
		"0.20", // critical_lower (20%)
		"1.00", // limit (100%)
		"0.01", // adjustment_rate_strained (1%)
		"0.10", // adjustment_rate_critical (10%)
	)

	tests := map[string]struct {
		balanceBefore               sdk.Coins
		balanceAfter                sdk.Coins
		rebalancingConfigs          map[string]cosmwasmpool.RebalancingConfig
		normalizationScalingFactors map[string]osmomath.Int
		assetGroups                 map[string]cosmwasmpool.AssetGroup
		expectedAdjustmentRate      osmomath.Dec
		expectedScaler              osmomath.Int
		description                 string
	}{
		"different normalization factors - demonstrates scaling": {
			balanceBefore: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(100_000)), // Lower USDC to start in strained zone
				sdk.NewCoin(USDT, osmomath.NewInt(900_000)), // Higher USDT
			),
			balanceAfter: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(50_000)),  // Move USDC even lower (to critical zone)
				sdk.NewCoin(USDT, osmomath.NewInt(950_000)), // USDT increases
			),
			rebalancingConfigs: map[string]cosmwasmpool.RebalancingConfig{
				pools.DenomPrefix + USDC: testRebalancingConfig,
				pools.DenomPrefix + USDT: testRebalancingConfig,
			},
			normalizationScalingFactors: map[string]osmomath.Int{
				USDC:               osmomath.NewInt(100), // USDC worth 100x more in normalized terms
				USDT:               osmomath.NewInt(1),   // USDT baseline
				OVERLY_PRECISE_USD: osmomath.NewInt(1),
				NO_PRECISION_USD:   osmomath.NewInt(1),
				ALLUSD:             osmomath.NewInt(1),
			},
			expectedAdjustmentRate: osmomath.MustNewDecFromStr("0.015419"), // Actual value from test
			expectedScaler:         osmomath.NewInt(10_900_000),            // max(before, after) with scaling
			description:            "Different normalization factors change the weighting significantly",
		},
		"scaler demonstration - smaller pool": {
			balanceBefore: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(50_000)),  // Smaller pool, start imbalanced
				sdk.NewCoin(USDT, osmomath.NewInt(450_000)), // 10% vs 90%
			),
			balanceAfter: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(25_000)), // Move to 5% vs 95% (more critical)
				sdk.NewCoin(USDT, osmomath.NewInt(475_000)),
			),
			rebalancingConfigs: map[string]cosmwasmpool.RebalancingConfig{
				pools.DenomPrefix + USDC: testRebalancingConfig,
				pools.DenomPrefix + USDT: testRebalancingConfig,
			},
			normalizationScalingFactors: map[string]osmomath.Int{
				USDC:               osmomath.NewInt(1),
				USDT:               osmomath.NewInt(1),
				OVERLY_PRECISE_USD: osmomath.NewInt(1),
				NO_PRECISION_USD:   osmomath.NewInt(1),
				ALLUSD:             osmomath.NewInt(1),
			},
			expectedAdjustmentRate: osmomath.MustNewDecFromStr("-0.01"), // Actual value from test run
			expectedScaler:         osmomath.NewInt(500_000),            // max(before, after) for harmful swap
			description:            "Smaller pool demonstrates proportional scaler with movement to critical zone",
		},
		"scaler demonstration - larger pool with same proportion": {
			balanceBefore: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(200_000)),   // 4x larger pool, same proportions
				sdk.NewCoin(USDT, osmomath.NewInt(1_800_000)), // 10% vs 90%
			),
			balanceAfter: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(100_000)), // Move to 5% vs 95% (same movement)
				sdk.NewCoin(USDT, osmomath.NewInt(1_900_000)),
			),
			rebalancingConfigs: map[string]cosmwasmpool.RebalancingConfig{
				pools.DenomPrefix + USDC: testRebalancingConfig,
				pools.DenomPrefix + USDT: testRebalancingConfig,
			},
			normalizationScalingFactors: map[string]osmomath.Int{
				USDC:               osmomath.NewInt(1),
				USDT:               osmomath.NewInt(1),
				OVERLY_PRECISE_USD: osmomath.NewInt(1),
				NO_PRECISION_USD:   osmomath.NewInt(1),
				ALLUSD:             osmomath.NewInt(1),
			},
			expectedAdjustmentRate: osmomath.MustNewDecFromStr("-0.01"), // Same adjustment as smaller pool
			expectedScaler:         osmomath.NewInt(2_000_000),          // 4x larger scaler
			description:            "Larger pool with same proportional movement shows larger scaler",
		},
		"normalization factor impact on scaler": {
			balanceBefore: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(500_000)), // Equal amounts
				sdk.NewCoin(USDT, osmomath.NewInt(500_000)),
			),
			balanceAfter: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(520_000)), // Small increase
				sdk.NewCoin(USDT, osmomath.NewInt(480_000)), // Small decrease
			),
			rebalancingConfigs: map[string]cosmwasmpool.RebalancingConfig{
				pools.DenomPrefix + USDC: testRebalancingConfig,
				pools.DenomPrefix + USDT: testRebalancingConfig,
			},
			normalizationScalingFactors: map[string]osmomath.Int{
				USDC:               osmomath.NewInt(5), // USDC worth 5x more
				USDT:               osmomath.NewInt(1), // USDT baseline
				OVERLY_PRECISE_USD: osmomath.NewInt(1),
				NO_PRECISION_USD:   osmomath.NewInt(1),
				ALLUSD:             osmomath.NewInt(1),
			},
			expectedAdjustmentRate: osmomath.MustNewDecFromStr("-0.002165"),
			expectedScaler:         osmomath.NewInt(3_080_000), // Weighted scaler: max(500*5+500*1, 520*5+480*1)
			description:            "Different normalization factors affect both weights and scaler calculation",
		},
		"asset group rebalancing configuration": {
			balanceBefore: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(350_000)),               // USDC in STABLE group (35%)
				sdk.NewCoin(OVERLY_PRECISE_USD, osmomath.NewInt(250_000)), // Also in STABLE group (25%) - total 60%
				sdk.NewCoin(USDT, osmomath.NewInt(400_000)),               // USDT 40%
			),
			balanceAfter: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(150_000)),               // USDC decreases to 15%
				sdk.NewCoin(OVERLY_PRECISE_USD, osmomath.NewInt(100_000)), // Also decreases to 10% - total STABLE 25% (below 30%)
				sdk.NewCoin(USDT, osmomath.NewInt(750_000)),               // USDT increases to 75% (above 70%)
			),
			rebalancingConfigs: map[string]cosmwasmpool.RebalancingConfig{
				pools.AssetGroupPrefix + "STABLE": testRebalancingConfig, // Asset group config
				pools.DenomPrefix + USDT:          testRebalancingConfig, // Individual denom config
			},
			assetGroups: map[string]cosmwasmpool.AssetGroup{
				"STABLE": {Denoms: []string{USDC, OVERLY_PRECISE_USD}},
			},
			normalizationScalingFactors: map[string]osmomath.Int{
				USDC:               osmomath.NewInt(1),
				USDT:               osmomath.NewInt(1),
				OVERLY_PRECISE_USD: osmomath.NewInt(1),
				NO_PRECISION_USD:   osmomath.NewInt(1),
				ALLUSD:             osmomath.NewInt(1),
			},
			expectedAdjustmentRate: osmomath.MustNewDecFromStr("-0.001"), // STABLE group and USDT both in strained zones
			expectedScaler:         osmomath.NewInt(1_000_000),           // max(before, after) for harmful swap
			description:            "Asset group rebalancing with combined weight calculation for grouped assets",
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			// Determine scaling factors to use - either custom from test or default
			scalingFactorsToUse := tc.normalizationScalingFactors

			// Create a routable pool with the test configuration
			routablePool := s.SetupRoutableAlloyTransmuterPoolCustom(
				USDC, USDT, tc.balanceBefore, osmomath.ZeroDec(),
				tc.rebalancingConfigs,
				cosmwasmpool.PrecomputedData{
					StdNormFactor:               osmomath.NewInt(1),
					NormalizationScalingFactors: scalingFactorsToUse,
				},
			)

			r := routablePool.(*pools.RoutableAlloyTransmuterPoolImpl)

			// Inject asset groups if provided by the test case
			if tc.assetGroups != nil {
				r.AlloyTransmuterData.AssetGroups = tc.assetGroups
			}

			// System under test
			adjustmentRate, scaler, err := r.ComputeTotalAdjustmentRate(tc.balanceBefore, tc.balanceAfter)

			s.Require().NoError(err, "ComputeTotalAdjustmentRate should not return error for test case: %s", tc.description)

			// Allow for small rounding differences (within 0.0001)
			tolerance := osmomath.MustNewDecFromStr("0.0001")
			diff := adjustmentRate.Sub(tc.expectedAdjustmentRate).Abs()
			s.Require().True(
				diff.LTE(tolerance),
				"Expected adjustment rate %s, got %s (diff: %s) for test case: %s",
				tc.expectedAdjustmentRate.String(),
				adjustmentRate.String(),
				diff.String(),
				tc.description,
			)

			s.Require().Equal(tc.expectedScaler, scaler, "Expected scaler %s, got %s for test case: %s", tc.expectedScaler.String(), scaler.String(), tc.description)
		})
	}
}
