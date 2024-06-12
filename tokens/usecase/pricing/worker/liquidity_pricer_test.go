package worker_test

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
)

var (

	// eth scaling factor 10^18
	ethScalingFactor = osmomath.MustNewDecFromStr("1000000000000000000")

	// 1
	oneScalingFactor = osmomath.OneDec()
)

// TestComputeCoinCap unit tests all valid and error cases of
// the ComputeCoinCap method of the liquidityPricer.
func (s *PoolLiquidityComputeWorkerSuite) TestComputeCoinCap() {
	var (
		priceOne = osmomath.OneBigDec()
		priceTwo = osmomath.NewBigDec(2)

		defaultAmount       = osmomath.NewInt(10)
		doubleDefaultAmount = defaultAmount.Add(defaultAmount)

		defaultResult       = defaultAmount.ToLegacyDec().Quo(defaultScalingFactor)
		doubleDefaultResult = defaultResult.MulInt64(2)

		zeroAmount = osmomath.ZeroInt()
	)

	tests := []struct {
		name               string
		coinAmount         osmomath.Int
		baseDenomPriceInfo domain.DenomPriceInfo

		expectedLiquidityCap osmomath.Dec
		expectedError        bool
	}{
		{
			name:       "default amount, price one, equal scaling factors",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceOne,
				ScalingFactor: defaultScalingFactor,
			},

			expectedLiquidityCap: defaultResult,
		},
		{
			name:       "double default amount, price one, equal scaling factors",
			coinAmount: doubleDefaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceOne,
				ScalingFactor: defaultScalingFactor,
			},

			expectedLiquidityCap: doubleDefaultResult,
		},
		{
			name:       "default amount, price two, equal scaling factors",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceTwo,
				ScalingFactor: defaultScalingFactor,
			},

			expectedLiquidityCap: doubleDefaultResult,
		},
		{
			name:       "default amount, price two, base scaling factor > quote scaling factor",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceTwo,
				ScalingFactor: ethScalingFactor,
			},

			expectedLiquidityCap: defaultAmount.ToLegacyDec().Quo(ethScalingFactor).Mul(priceTwo.Dec()),
		},
		{
			name:       "default amount, price two, base scaling factor < quote scaling factor",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceTwo,
				ScalingFactor: oneScalingFactor,
			},

			expectedLiquidityCap: defaultAmount.MulRaw(2).ToLegacyDec(),
		},

		{
			name:       "zero amount, price one, one scaling factor",
			coinAmount: zeroAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceOne,
				ScalingFactor: oneScalingFactor,
			},

			expectedLiquidityCap: osmomath.ZeroDec(),
		},

		// Error cases
		{
			name:       "error: zero price",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         osmomath.ZeroBigDec(),
				ScalingFactor: oneScalingFactor,
			},

			expectedError: true,
		},

		{
			name:       "error: zero base scaling factor",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceOne,
				ScalingFactor: osmomath.ZeroDec(),
			},

			expectedError: true,
		},
		{
			name: "error: truncation",
			// coinAmount * price / (quoteScalingFactor / baseScalingFactor)
			// 1 * 10^-36 / 10^12 => below the precision of 36
			coinAmount: osmomath.OneInt(),
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         osmomath.SmallestBigDec(),
				ScalingFactor: ethScalingFactor,
			},

			expectedError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// System under test
			usdcLiquidity, err := worker.ComputeCoinCap(sdk.NewCoin(UOSMO, tt.coinAmount), tt.baseDenomPriceInfo)
			if tt.expectedError {
				s.Require().Error(err)
				return
			}

			s.Require().NoError(err)

			s.Require().Equal(tt.expectedLiquidityCap, usdcLiquidity)
		})
	}
}

// TestComputeCoinCap_AllCoin tests the ComputeCoinCap method for all listed coins
// by estimating 10^6 amount in terms of USDC using mainnet state.
// This test is skipped by default but is kept for ease of debugging in case we start
// having edge cases failures for some denom pairs. In that case, we can quickly
// run this test and debug.
func (s *PoolLiquidityComputeWorkerSuite) TestPriceCoin_AllCoin() {
	s.T().Parallel()
	s.T().Skip("skipping long-running test by default. To be used in cases where we need to identify breakages across all coins")

	// Set up mainnet mock state.
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata()
	s.Require().NoError(err)

	baseDenoms := make([]string, 0, len(tokenMetadata))
	for chainDenom := range tokenMetadata {
		baseDenoms = append(baseDenoms, chainDenom)
	}

	quoteChainDenom, err := mainnetUsecase.Tokens.GetChainDenom(defaultPricingConfig.DefaultQuoteHumanDenom)
	s.Require().NoError(err)

	prices, err := mainnetUsecase.Tokens.GetPrices(context.TODO(), baseDenoms, []string{quoteChainDenom}, domain.ChainPricingSourceType, domain.WithMinPricingPoolLiquidityCap(0))
	s.Require().NoError(err)

	type errorData struct {
		humanDenom string
		err        error
	}

	errors := make([]errorData, 0)

	s.Require().NotZero(len(tokenMetadata))
	for chainDenom, token := range tokenMetadata {
		chainAmount := osmomath.NewDec(10).PowerMut(uint64(token.Precision + 1)).TruncateInt()

		baseDenomPrices, ok := prices[chainDenom]
		s.Require().True(ok)

		baseQuotePrice, ok := baseDenomPrices[quoteChainDenom]
		s.Require().True(ok)

		scalingFactor, err := mainnetUsecase.Tokens.GetChainScalingFactorByDenomMut(chainDenom)
		s.Require().NoError(err)

		baseDenomPriceInfo := domain.DenomPriceInfo{
			Price:         baseQuotePrice,
			ScalingFactor: scalingFactor,
		}

		// System under test
		usdcLiquidity, err := worker.ComputeCoinCap(sdk.NewCoin(chainDenom, chainAmount), baseDenomPriceInfo)

		if err != nil {
			errors = append(errors, errorData{
				humanDenom: token.HumanDenom,
				err:        err,
			})
		} else {
			fmt.Printf("Liquidity for 10 %s in %s: %s\n", token.HumanDenom, defaultPricingConfig.DefaultQuoteHumanDenom, usdcLiquidity.String())

			// Sanity check that stables are roughly equal to 10
		}
	}

	fmt.Printf("\n\nErrors:\n")
	for _, err := range errors {
		fmt.Printf("denom: %s, error: %s\n", err.humanDenom, err.err.Error())
	}
}

// TestPriceCoin tests the TestPriceCoin method by following the spec.
func (s *PoolLiquidityComputeWorkerSuite) TestPriceCoin() {
	var (
		ethScaledLiquidity = ethScalingFactor.MulInt(defaultLiquidity).TruncateInt()

		defaultPriceOne = osmomath.OneBigDec()
	)

	tests := []struct {
		name string

		preSetScalingFactorDenom string
		preSetScalingFactorValue osmomath.Dec

		denom          string
		totalLiquidity osmomath.Int
		price          osmomath.BigDec

		expectedCapitalization osmomath.Int
	}{
		{
			name: "scaling factor unset",

			denom:          UOSMO,
			totalLiquidity: defaultLiquidity,
			price:          defaultPriceOne,

			expectedCapitalization: zeroCapitalization,
		},
		{
			name: "zero price -> produces zero capitalization",

			preSetScalingFactorDenom: UOSMO,
			preSetScalingFactorValue: defaultScalingFactor,

			denom:          UOSMO,
			totalLiquidity: defaultLiquidity,
			price:          osmomath.ZeroBigDec(),

			expectedCapitalization: zeroCapitalization,
		},
		{
			name: "truncate -> produces zero capitalization",

			// totalLiquidity * price / (quoteScalingFactor / baseScalingFactor)
			// 1 * 10^-36 / 10^18 => below the precision of 36
			preSetScalingFactorDenom: UOSMO,
			preSetScalingFactorValue: ethScalingFactor,

			denom:          UOSMO,
			totalLiquidity: osmomath.OneInt(),
			price:          osmomath.SmallestBigDec(),

			expectedCapitalization: zeroCapitalization,
		},
		{
			name: "happy path",

			preSetScalingFactorDenom: UOSMO,
			preSetScalingFactorValue: defaultScalingFactor,

			denom:          UOSMO,
			totalLiquidity: defaultLiquidity,
			price:          defaultPriceOne,

			// 10^6 / 10^6 = 1
			expectedCapitalization: osmomath.OneInt(),
		},
		{
			name: "happy path with different inputs",

			preSetScalingFactorDenom: ATOM,
			preSetScalingFactorValue: ethScalingFactor,

			denom:          ATOM,
			totalLiquidity: ethScaledLiquidity.MulRaw(2),
			price:          osmomath.NewBigDec(2),

			expectedCapitalization: ethScaledLiquidity.ToLegacyDec().QuoMut(ethScalingFactor).TruncateInt().MulRaw(4),
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

			scalingFactorGetterCbMock := mocks.SetupMockScalingFactorCb(tt.denom, tt.preSetScalingFactorValue, nil)

			// Create liquidity pricer
			liquidityPricer := worker.NewLiquidityPricer(USDC, scalingFactorGetterCbMock)

			// System under test
			liquidityCapitalization := liquidityPricer.PriceCoin(sdk.NewCoin(tt.denom, tt.totalLiquidity), tt.price)

			// Check the result
			s.Require().Equal(tt.expectedCapitalization.String(), liquidityCapitalization.String())
		})
	}
}

// TestPriceBalances tests the TestPriceBalances method by following the spec.
func (s *PoolLiquidityComputeWorkerSuite) TestPriceBalances() {
	const (
		noErrorStr = ""
	)

	var (
		defaultCoin = sdk.NewCoin(UOSMO, defaultLiquidity)
		secondCoin  = sdk.NewCoin(ATOM, defaultLiquidity)
	)

	tests := []struct {
		name string

		preSetScalingFactorMap map[string]osmomath.Dec

		balances sdk.Coins
		prices   domain.PricesResult

		expectedLiquidityCap osmomath.Int
		errorStr             string
	}{
		{
			name: "single coin happy path",

			preSetScalingFactorMap: defaultScalingFactorMap,
			balances:               sdk.NewCoins(defaultCoin),

			prices: defaultBlockPriceUpdates,

			expectedLiquidityCap: defaultLiquidityCap,
			errorStr:             noErrorStr,
		},
		{
			name: "single coin no price -> error set",

			preSetScalingFactorMap: defaultScalingFactorMap,
			balances:               sdk.NewCoins(defaultCoin),

			prices: domain.PricesResult{},

			expectedLiquidityCap: osmomath.ZeroInt(),
			errorStr:             worker.FormatLiquidityCapErrorStr(UOSMO),
		},
		{
			name: "two coin happy path",

			preSetScalingFactorMap: defaultScalingFactorMap,
			balances:               sdk.NewCoins(defaultCoin, secondCoin),

			prices: defaultBlockPriceUpdates,

			expectedLiquidityCap: defaultLiquidityCap.Add(defaultLiquidityCap),
			errorStr:             noErrorStr,
		},
		{
			name: "one of the coins no price -> another coin still contributes, error set",

			preSetScalingFactorMap: defaultScalingFactorMap,
			balances:               sdk.NewCoins(defaultCoin, secondCoin),

			// Note: no price for ATOM
			prices: domain.PricesResult{
				UOSMO: {
					USDC: defaultPrice,
				},
			},

			expectedLiquidityCap: defaultLiquidityCap,
			errorStr:             worker.FormatLiquidityCapErrorStr(ATOM),
		},
		{
			name: "two coin both error",

			preSetScalingFactorMap: defaultScalingFactorMap,
			balances:               sdk.NewCoins(defaultCoin, secondCoin),

			// Note: no prices for both tokens
			prices: domain.PricesResult{},

			expectedLiquidityCap: zeroCapitalization,
			errorStr:             worker.FormatLiquidityCapErrorStr(ATOM) + worker.LiquidityCapErrorSeparator + worker.FormatLiquidityCapErrorStr(UOSMO),
		},
	}

	for _, tc := range tests {
		tc := tc

		s.T().Run("", func(t *testing.T) {

			scalingFactorGetterMock := mocks.SetupMockScalingFactorCbFromMap(tc.preSetScalingFactorMap)

			// Create liquidity pricer
			liquidityPricer := worker.NewLiquidityPricer(USDC, scalingFactorGetterMock)

			liquidityCap, errStr := liquidityPricer.PriceBalances(tc.balances, tc.prices)

			s.Require().Equal(tc.expectedLiquidityCap.String(), liquidityCap.String())
			s.Require().Equal(tc.errorStr, errStr)
		})
	}
}
