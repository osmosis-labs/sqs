package workers_test

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/pools/usecase/workers"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
	"github.com/stretchr/testify/suite"
)

type PoolLiquidityUpdateWorkerSuite struct {
	routertesting.RouterTestHelper
}

var (
	defaultPricingRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:  []uint64{},
		MaxRoutes:         5,
		MaxPoolsPerRoute:  3,
		MaxSplitRoutes:    3,
		MinOSMOLiquidity:  50,
		RouteCacheEnabled: true,
	}

	pricingCacheExpiry = 2000

	defaultPricingConfig = domain.PricingConfig{
		DefaultSource:          domain.ChainPricingSource,
		CacheExpiryMs:          pricingCacheExpiry,
		DefaultQuoteHumanDenom: "usdc",
		MaxPoolsPerRoute:       4,
		MaxRoutes:              5,
		MinOSMOLiquidity:       0,
	}
)

var (
	stableCoinDenoms = []string{"usdc", "usdt", "dai", "ist"}
)

func TestPoolLiquidityUpdateWorkerSuite(t *testing.T) {
	suite.Run(t, new(PoolLiquidityUpdateWorkerSuite))
}

// TODO: improve test
func (s *PoolLiquidityUpdateWorkerSuite) TestComputeCoinTVL() {

	s.T().Parallel()
	// s.T().Skip("skipping test")

	// Set up mainnet mock state.
	pricingRouterConfig := defaultPricingRouterConfig
	pricingConfig := defaultPricingConfig
	pricingRouterConfig.MinOSMOLiquidity = 0
	pricingConfig.MinOSMOLiquidity = 1
	router, mainnetState := s.SetupMainnetRouter(pricingRouterConfig, pricingConfig)
	mainnetState.PricingConfig = defaultPricingConfig
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata(s.Ctx)
	s.Require().NoError(err)

	baseDenoms := make([]string, 0, len(tokenMetadata))
	for chainDenom := range tokenMetadata {
		baseDenoms = append(baseDenoms, chainDenom)
	}

	quoteChainDenom, err := mainnetUsecase.Tokens.GetChainDenom(context.TODO(), defaultPricingConfig.DefaultQuoteHumanDenom)
	s.Require().NoError(err)

	chainPricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	prices, err := mainnetUsecase.Tokens.GetPrices(context.TODO(), baseDenoms, []string{quoteChainDenom}, chainPricingStrategy)
	s.Require().NoError(err)

	type errorData struct {
		humanDenom string
		err        error
	}

	errors := make([]errorData, 0)

	s.Require().NotZero(len(tokenMetadata))
	for chainDenom, token := range tokenMetadata {
		if token.HumanDenom == "USDC" {
			fmt.Println("here")
		}

		chainAmount := osmomath.NewDec(10).PowerMut(uint64(token.Precision + 1)).TruncateInt()

		baseDenomPrices, ok := prices[chainDenom]
		s.Require().True(ok)

		baseQuotePriceAny, ok := baseDenomPrices[quoteChainDenom]
		s.Require().True(ok)

		baseQuotePrice, ok := baseQuotePriceAny.(osmomath.BigDec)
		s.Require().True(ok)

		scalingFactor, err := mainnetUsecase.Tokens.GetChainScalingFactorByDenomMut(context.TODO(), chainDenom)
		s.Require().NoError(err)

		baseDenomPriceInfo := workers.DenomPriceInfo{
			Price:         baseQuotePrice,
			ScalingFactor: scalingFactor,
		}

		// System under test
		usdcLiquidity, err := workers.ComputeCoinTVL(sdk.NewCoin(chainDenom, chainAmount), baseDenomPriceInfo)

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
