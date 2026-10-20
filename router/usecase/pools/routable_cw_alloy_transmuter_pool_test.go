package pools_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/v31/ingest/types/cosmwasmpool"
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
	// Note: empty precomputed data and rate limiter config
	return s.SetupRoutableAlloyTransmuterPoolCustom(tokenInDenom, tokenOutDenom, balances, takerFee, cosmwasmpool.AlloyedRateLimiter{}, cosmwasmpool.PrecomputedData{})
}

func (s *RoutablePoolTestSuite) SetupRoutableAlloyTransmuterPoolCustom(tokenInDenom, tokenOutDenom string, balances sdk.Coins, takerFee osmomath.Dec, rateLimitConfig cosmwasmpool.AlloyedRateLimiter, preComputedData cosmwasmpool.PrecomputedData) domain.RoutablePool {
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

					RateLimiterConfig: rateLimitConfig,
					PreComputedData:   preComputedData,
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

// Tests exact amount out quotes through an alloyed transmuter whose assets have differing
// normalization factors (USDT 1, ALLUSD 10, USDC 100), in both directions. The token in is
// rounded up as the contract does, so it is the smallest amount whose out-given-in quote
// covers the requested token out.
func (s *RoutablePoolTestSuite) TestCalculateTokenInByTokenOut_AlloyTransmuter() {
	defaultBalanceAmt := osmomath.NewInt(1_000_000)
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, defaultBalanceAmt), sdk.NewCoin(USDT, defaultBalanceAmt))

	// Normalized balances are balance * scaling factor (standard normalization factor 100).
	scalingFactors := map[string]osmomath.Int{
		USDC:               osmomath.NewInt(1),
		USDT:               osmomath.NewInt(100),
		OVERLY_PRECISE_USD: osmomath.NewInt(1),
		NO_PRECISION_USD:   osmomath.NewInt(1),
	}
	limiterBalances := sdk.NewCoins(sdk.NewCoin(USDC, osmomath.NewInt(100_000_000)), sdk.NewCoin(USDT, defaultBalanceAmt))

	tests := map[string]struct {
		tokenOut        sdk.Coin
		tokenInDenom    string
		balances        sdk.Coins
		staticLimiters  map[string]cosmwasmpool.StaticLimiter
		expectedTokenIn osmomath.Int
		expectError     error
	}{
		"variant to alloyed": {
			tokenOut:        sdk.NewCoin(ALLUSD, osmomath.NewInt(1001)),
			tokenInDenom:    USDT,
			balances:        defaultBalances,
			expectedTokenIn: osmomath.NewInt(101), // 1001 * 1 / 10 = 100.1, rounded up
		},
		"variant to alloyed: token in balance is not required": {
			tokenOut:        sdk.NewCoin(ALLUSD, osmomath.NewInt(1001)),
			tokenInDenom:    USDT,
			balances:        sdk.NewCoins(sdk.NewCoin(USDC, defaultBalanceAmt)),
			expectedTokenIn: osmomath.NewInt(101),
		},
		"alloyed to variant": {
			tokenOut:        sdk.NewCoin(USDC, osmomath.NewInt(1001)),
			tokenInDenom:    ALLUSD,
			balances:        defaultBalances,
			expectedTokenIn: osmomath.NewInt(101), // 1001 * 10 / 100 = 100.1, rounded up
		},
		"variant to variant": {
			tokenOut:        sdk.NewCoin(USDC, osmomath.NewInt(1001)),
			tokenInDenom:    USDT,
			balances:        defaultBalances,
			expectedTokenIn: osmomath.NewInt(11), // 1001 * 1 / 100 = 10.01, rounded up
		},
		"error: token out is larger than balance of token out": {
			tokenOut:     sdk.NewCoin(USDC, defaultBalanceAmt.AddRaw(1)),
			tokenInDenom: ALLUSD,
			balances:     defaultBalances,
			expectError: domain.TransmuterInsufficientBalanceError{
				Denom:         USDC,
				BalanceAmount: defaultBalanceAmt.String(),
				Amount:        defaultBalanceAmt.AddRaw(1).String(),
			},
		},
		"error: token in exceeds its static limiter": {
			// USDC in = 500_000 * 100 = 50_000_000.
			// Normalized after: USDC 150_000_000, USDT 500_000 * 100 = 50_000_000, so USDC weight 0.75.
			tokenOut:       sdk.NewCoin(USDT, osmomath.NewInt(500_000)),
			tokenInDenom:   USDC,
			balances:       limiterBalances,
			staticLimiters: map[string]cosmwasmpool.StaticLimiter{USDC: {UpperLimit: "0.6"}},
			expectError: domain.StaticRateLimiterInvalidUpperLimitError{
				UpperLimit: "0.6",
				Weight:     osmomath.MustNewDecFromStr("0.75").String(),
				Denom:      USDC,
			},
		},
		"token in within its static limiter": {
			// Normalized after: USDC 110_000_000, USDT 90_000_000, so USDC weight 0.55.
			tokenOut:        sdk.NewCoin(USDT, osmomath.NewInt(100_000)),
			tokenInDenom:    USDC,
			balances:        limiterBalances,
			staticLimiters:  map[string]cosmwasmpool.StaticLimiter{USDC: {UpperLimit: "0.6"}},
			expectedTokenIn: osmomath.NewInt(10_000_000),
		},
		"static limiter is skipped when the swap drains all underlying assets": {
			// 1_000_000 * 10 / 100 = 100_000 ALLUSD in, leaving no underlying assets in the pool.
			// USDT's limiter would be checked since its weight rises as USDC leaves.
			tokenOut:        sdk.NewCoin(USDC, defaultBalanceAmt),
			tokenInDenom:    ALLUSD,
			balances:        sdk.NewCoins(sdk.NewCoin(USDC, defaultBalanceAmt)),
			staticLimiters:  map[string]cosmwasmpool.StaticLimiter{USDT: {UpperLimit: "0.6"}},
			expectedTokenIn: osmomath.NewInt(100_000),
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableAlloyTransmuterPoolCustom(tc.tokenInDenom, tc.tokenOut.Denom, tc.balances, osmomath.ZeroDec(), cosmwasmpool.AlloyedRateLimiter{
				StaticLimiterByDenomMap: tc.staticLimiters,
			}, cosmwasmpool.PrecomputedData{
				StdNormFactor:               osmomath.NewInt(100),
				NormalizationScalingFactors: scalingFactors,
			})

			tokenIn, err := routablePool.CalculateTokenInByTokenOut(context.TODO(), tc.tokenOut)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectError)
				return
			}
			s.Require().NoError(err)

			s.Require().Equal(tc.tokenInDenom, tokenIn.Denom)
			s.Require().Equal(tc.expectedTokenIn.String(), tokenIn.Amount.String())

			// Round trip: the quoted token in buys at least the token out, and one unit less does not.
			tokenOut, err := routablePool.CalculateTokenOutByTokenIn(context.TODO(), tokenIn)
			s.Require().NoError(err)
			s.Require().True(tokenOut.Amount.GTE(tc.tokenOut.Amount), "out given in %s is less than %s", tokenOut, tc.tokenOut)

			tokenOut, err = routablePool.CalculateTokenOutByTokenIn(context.TODO(), sdk.NewCoin(tokenIn.Denom, tokenIn.Amount.SubRaw(1)))
			s.Require().NoError(err)
			s.Require().True(tokenOut.Amount.LT(tc.tokenOut.Amount), "one unit less of token in still buys %s", tokenOut)
		})
	}
}

// Tests that spot price ignores the static rate limiter, as the contract's spot price query does.
// Adding one unit of USDC would push its weight to 601 / 1001, over the 0.6 limit.
func (s *RoutablePoolTestSuite) TestCalcSpotPrice_AlloyTransmuter_IgnoresStaticLimiter() {
	s.Setup()
	balances := sdk.NewCoins(sdk.NewCoin(USDC, osmomath.NewInt(600)), sdk.NewCoin(USDT, osmomath.NewInt(400)))
	routablePool := s.SetupRoutableAlloyTransmuterPoolCustom(USDC, ALLUSD, balances, osmomath.ZeroDec(), cosmwasmpool.AlloyedRateLimiter{
		StaticLimiterByDenomMap: map[string]cosmwasmpool.StaticLimiter{USDC: {UpperLimit: "0.6"}},
	}, cosmwasmpool.PrecomputedData{
		StdNormFactor: osmomath.OneInt(),
		NormalizationScalingFactors: map[string]osmomath.Int{
			USDC:               osmomath.OneInt(),
			USDT:               osmomath.OneInt(),
			OVERLY_PRECISE_USD: osmomath.OneInt(),
			NO_PRECISION_USD:   osmomath.OneInt(),
		},
	})

	spotPrice, err := routablePool.CalcSpotPrice(context.TODO(), USDC, ALLUSD)
	s.Require().NoError(err)
	s.Require().Equal(osmomath.MustNewBigDecFromStr("0.1"), spotPrice) // 1 * 10 / 100
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
		tokenOut        sdk.Coin
		tokenInDenom    string
		expectedTokenIn osmomath.Int
		expectedError   error
	}{
		"valid calculation using normalization factors": {
			tokenOut:        sdk.NewCoin(USDC, osmomath.NewInt(100)),
			tokenInDenom:    USDT,
			expectedTokenIn: osmomath.NewInt(1), // (100 * 1) / 100 = 1
			expectedError:   nil,
		},
		"valid calculation with decimal points, rounded up": {
			tokenOut:        sdk.NewCoin(USDC, osmomath.NewInt(10)),
			tokenInDenom:    USDT,
			expectedTokenIn: osmomath.NewInt(1), // (10 * 1) / 100 = 0.1, rounded up
			expectedError:   nil,
		},
		"valid calculation, rounded up from near zero": {
			tokenOut:        sdk.NewCoin(OVERLY_PRECISE_USD, osmomath.NewInt(10)),
			tokenInDenom:    USDC,
			expectedTokenIn: osmomath.NewInt(1),
			expectedError:   nil,
		},
		"missing normalization factor for token in": {
			tokenOut:      sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(100)),
			tokenInDenom:  USDT,
			expectedError: domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
		"missing normalization factor for token out": {
			tokenOut:      sdk.NewCoin(USDC, osmomath.NewInt(100)),
			tokenInDenom:  INVALID_DENOM,
			expectedError: domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
		},
		"missing normalization factors for both token in and token out": {
			tokenOut:      sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(100)),
			tokenInDenom:  INVALID_DENOM,
			expectedError: domain.MissingNormalizationFactorError{Denom: INVALID_DENOM, PoolId: defaultPoolID},
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
				s.Require().Equal(tc.expectedTokenIn.String(), tokenIn.String())
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

	defaultStaticLimiterConfig := map[string]cosmwasmpool.StaticLimiter{
		USDC: {UpperLimit: "0.5"},
	}

	tests := map[string]struct {
		tokenInCoin                 sdk.Coin
		tokenOutDenom               string
		initialBalances             sdk.Coins
		standardNormFactor          osmomath.Int
		normalizationScalingFactors map[string]osmomath.Int
		staticLimiterConfig         map[string]cosmwasmpool.StaticLimiter
		expectError                 error
	}{
		"valid token in - below upper limit": {
			tokenInCoin:                 sdk.NewCoin(USDC, osmomath.NewInt(100_000)),
			tokenOutDenom:               USDT,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			staticLimiterConfig:         defaultStaticLimiterConfig,
			expectError:                 nil,
		},
		"invalid token in - exceeds upper limit": {
			tokenInCoin:                 sdk.NewCoin(USDC, osmomath.NewInt(2_000_000)),
			tokenOutDenom:               USDT,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			staticLimiterConfig:         defaultStaticLimiterConfig,
			expectError: domain.StaticRateLimiterInvalidUpperLimitError{
				Denom:      USDC,
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
			staticLimiterConfig:         map[string]cosmwasmpool.StaticLimiter{},
			expectError:                 nil,
		},
		"static limiter not set for token in denom": {
			tokenInCoin:                 sdk.NewCoin(USDT, osmomath.NewInt(1_000_000)),
			tokenOutDenom:               USDC,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			staticLimiterConfig:         defaultStaticLimiterConfig,
			expectError:                 nil,
		},
		"check all assets' static limiters when token in denom is alloyed": {
			tokenInCoin:                 sdk.NewCoin(ALLUSD, osmomath.NewInt(1_000_001)),
			tokenOutDenom:               USDT,
			initialBalances:             defaultInitialBalances,
			standardNormFactor:          defaultStandardNormFactor,
			normalizationScalingFactors: defaultScalingFactors,
			staticLimiterConfig:         defaultStaticLimiterConfig,
			expectError: domain.StaticRateLimiterInvalidUpperLimitError{
				Denom:      USDC,
				UpperLimit: "0.5",
				Weight:     osmomath.MustNewDecFromStr("0.500000250000125").String(),
			},
		},
		"different normalization factors": {
			tokenInCoin:   sdk.NewCoin(USDC, osmomath.NewInt(500_000)),
			tokenOutDenom: USDT,
			initialBalances: sdk.NewCoins(
				sdk.NewCoin(USDC, osmomath.NewInt(1_000_000)),
				sdk.NewCoin(USDT, osmomath.NewInt(2_000_000)),
			),
			standardNormFactor: defaultStandardNormFactor,
			normalizationScalingFactors: map[string]osmomath.Int{
				USDC:               osmomath.NewInt(2),
				USDT:               oneInt,
				OVERLY_PRECISE_USD: oneInt,
				NO_PRECISION_USD:   oneInt,
			},
			staticLimiterConfig: map[string]cosmwasmpool.StaticLimiter{
				USDC: {UpperLimit: "0.7"},
			},
			expectError: nil,
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableAlloyTransmuterPoolCustom(tc.tokenInCoin.Denom, tc.tokenOutDenom, tc.initialBalances, osmomath.ZeroDec(), cosmwasmpool.AlloyedRateLimiter{
				StaticLimiterByDenomMap: tc.staticLimiterConfig,
			}, cosmwasmpool.PrecomputedData{
				StdNormFactor:               tc.standardNormFactor,
				NormalizationScalingFactors: tc.normalizationScalingFactors,
			})

			r := routablePool.(*pools.RoutableAlloyTransmuterPoolImpl)

			// System under test
			// The token out amount equals the token in amount, as these cases use equal scaling factors
			// for the swapped assets.
			err := r.CheckStaticRateLimiter(tc.tokenInCoin, sdk.NewCoin(tc.tokenOutDenom, tc.tokenInCoin.Amount))

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectError)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}
