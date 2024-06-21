package worker_test

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
)

var (
	// 10^6
	defaultQuoteDenomScalingFactor = osmomath.MustNewDecFromStr("1000000")

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
				ScalingFactor: defaultQuoteDenomScalingFactor,
			},

			expectedLiquidityCap: defaultAmount.ToLegacyDec(),
		},
		{
			name:       "double default amount, price one, equal scaling factors",
			coinAmount: doubleDefaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceOne,
				ScalingFactor: defaultQuoteDenomScalingFactor,
			},

			expectedLiquidityCap: doubleDefaultAmount.ToLegacyDec(),
		},
		{
			name:       "default amount, price two, equal scaling factors",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceTwo,
				ScalingFactor: defaultQuoteDenomScalingFactor,
			},

			expectedLiquidityCap: osmomath.NewDecFromInt(doubleDefaultAmount),
		},
		{
			name:       "default amount, price two, base scaling factor > quote scaling factor",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceTwo,
				ScalingFactor: ethScalingFactor,
			},

			expectedLiquidityCap: osmomath.NewDecFromInt(doubleDefaultAmount).Mul(defaultQuoteDenomScalingFactor).Quo(ethScalingFactor),
		},
		{
			name:       "default amount, price two, base scaling factor < quote scaling factor",
			coinAmount: defaultAmount,
			baseDenomPriceInfo: domain.DenomPriceInfo{
				Price:         priceTwo,
				ScalingFactor: oneScalingFactor,
			},

			expectedLiquidityCap: osmomath.NewDecFromInt(doubleDefaultAmount).Mul(defaultQuoteDenomScalingFactor).Quo(oneScalingFactor),
		},

		{
			name:       "zero amount, price one, equal scaling factors",
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

			// Liquidity pricer
			liquidityPricer := worker.NewLiquidityPricer(USDC, defaultQuoteDenomScalingFactor)

			// System under test
			usdcLiquidity, err := liquidityPricer.ComputeCoinCap(sdk.NewCoin(UOSMO, tt.coinAmount), tt.baseDenomPriceInfo)
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
func (s *PoolLiquidityComputeWorkerSuite) TestComputeCoinCap_AllCoin() {
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

	liquidityPricer := worker.NewLiquidityPricer(USDC, defaultQuoteDenomScalingFactor)

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
		usdcLiquidity, err := liquidityPricer.ComputeCoinCap(sdk.NewCoin(chainDenom, chainAmount), baseDenomPriceInfo)

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
