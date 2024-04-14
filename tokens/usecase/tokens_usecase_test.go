package usecase_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"
)

type TokensUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultCosmosExponent     = 6
	ethExponent               = 18
	defaultPricingCacheExpiry = time.Second * 2

	mainnetAssetListFileURL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/osmosis-1.assetlist.json"
)

var (
	UOSMO   = routertesting.UOSMO
	ATOM    = routertesting.ATOM
	stOSMO  = routertesting.STOSMO
	stATOM  = routertesting.STATOM
	USDC    = routertesting.USDC
	USDCaxl = routertesting.USDCaxl
	USDT    = routertesting.USDT
	WBTC    = routertesting.WBTC
	ETH     = routertesting.ETH
	AKT     = routertesting.AKT
	UMEE    = routertesting.UMEE
	UION    = routertesting.UION
	CRE     = routertesting.CRE

	defaultPricingRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:  []uint64{},
		MaxRoutes:         5,
		MaxPoolsPerRoute:  3,
		MaxSplitRoutes:    3,
		MinOSMOLiquidity:  50,
		RouteCacheEnabled: true,
	}

	defaultPricingConfig = domain.PricingConfig{
		DefaultSource:          domain.ChainPricingSourceType,
		CacheExpiryMs:          pricingCacheExpiry,
		DefaultQuoteHumanDenom: "usdc",
		MaxPoolsPerRoute:       4,
		MaxRoutes:              5,
		MinOSMOLiquidity:       50,
	}
)

func TestTokensUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(TokensUseCaseTestSuite))
}

func (s *TokensUseCaseTestSuite) TestParseAssetList() {
	env := os.Getenv("CI_SQS_ASSETLIST_TEST")
	if env != "true" {
		s.T().Skip("skip the test that does network call and is used for debugging")
	}

	tokensMap, err := tokensusecase.GetTokensFromChainRegistry(mainnetAssetListFileURL)
	s.Require().NoError(err)
	s.Require().NotEmpty(tokensMap)

	// ATOM is present
	atomToken, ok := tokensMap[ATOM]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, atomToken.Precision)

	// ION is present
	ionMainnetDenom := "uion"
	ionToken, ok := tokensMap[ionMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, ionToken.Precision)

	// IBCX is present
	ibcxMainnetDenom := "factory/osmo14klwqgkmackvx2tqa0trtg69dmy0nrg4ntq4gjgw2za4734r5seqjqm4gm/uibcx"
	ibcxToken, ok := tokensMap[ibcxMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, ibcxToken.Precision)

	// DYSON is present
	dysonMainnetDenom := "ibc/E27CD305D33F150369AB526AEB6646A76EC3FFB1A6CA58A663B5DE657A89D55D"
	dysonToken, ok := tokensMap[dysonMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(0, dysonToken.Precision)

	// ETH is present
	ethToken, ok := tokensMap[ETH]
	s.Require().True(ok)
	s.Require().Equal(ethExponent, ethToken.Precision)
}

func (s *TokensUseCaseTestSuite) TestParseExponents_Testnet() {
	s.T().Skip("skip the test that does network call and is used for debugging")

	const (
		testnetAssetListFileURL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmo-test-5/osmo-test-5.assetlist.json"
	)
	tokensMap, err := tokensusecase.GetTokensFromChainRegistry(testnetAssetListFileURL)
	s.Require().NoError(err)
	s.Require().NotEmpty(tokensMap)

	// uosmo is present
	osmoToken, ok := tokensMap[UOSMO]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, osmoToken.Precision)
}

// This test validates that on-chain pricing works as intended.
//
// It sets up mock mainnet state.
//
// Next, it gets prices with USDC and USDT as quotes for several top denoms.
//
// It iterates over results and confirms that, for each denom, the difference is at most 1%.
//
// Additionally, for sanit check it confirms that for WBTC / USDC the price is within 15% of 50K
// (approximately the real price at the time of writing)
func (s *TokensUseCaseTestSuite) TestGetPrices_Chain() {

	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(defaultPricingRouterConfig, defaultPricingConfig)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	// System under test.
	prices, err := mainnetUsecase.Tokens.GetPrices(context.Background(), routertesting.MainnetDenoms, []string{USDC, USDT})
	s.Require().NoError(err)

	errTolerance := osmomath.ErrTolerance{
		// 1% tolerance
		MultiplicativeTolerance: osmomath.MustNewDecFromStr("0.01"),
	}

	// For each base denom, validate that its USDC and USDT prices differ by at most
	// 1%
	s.Require().Len(prices, len(routertesting.MainnetDenoms))
	for _, baseAssetPrices := range prices {
		// USDC and USDT
		s.Require().Len(baseAssetPrices, 2)

		usdcQuoteAny, ok := baseAssetPrices[USDC]
		s.Require().True(ok)
		usdcQuote := s.ConvertAnyToBigDec(usdcQuoteAny)

		usdtQuoteAny, ok := baseAssetPrices[USDT]
		s.Require().True(ok)
		usdtQuote := s.ConvertAnyToBigDec(usdtQuoteAny)

		result := errTolerance.CompareBigDec(usdcQuote, usdtQuote)
		s.Require().Zero(result, fmt.Sprintf("usdcQuote: %s, usdtQuote: %s", usdcQuote, usdtQuote))
	}

	// WBTC is around 50K at the time of creation of this test
	// We set tolerance to 15% and compare against this value to have sanity checks
	// in place against a hardcoded expected value rather than comparing USDT and USDC prices only
	// that are both computed by the system.
	// Noe: if WBTC price changes by more than 15% and we update test mainnet state, this test is likely to fail.
	expectedwBTCPrice := osmomath.NewBigDec(70000)
	wbtcErrorTolerance := osmomath.ErrTolerance{
		// 15% tolerance
		MultiplicativeTolerance: osmomath.MustNewDecFromStr("0.15"),
	}

	actualwBTCUSDCPriceAny, ok := prices[WBTC][USDC]
	s.Require().True(ok)
	actualwBTCUSDCPrice := s.ConvertAnyToBigDec(actualwBTCUSDCPriceAny)

	result := wbtcErrorTolerance.CompareBigDec(expectedwBTCPrice, actualwBTCUSDCPrice)
	s.Require().Zero(result)
}

// We use this test in CI for detecting tokens with unsupported pricing.
// The config used is the `config.json` in root which is expected to be as close
// to mainnet as possible.
//
// The mainnet state must be manually updated when needed with 'make sqs-update-mainnet-state'
func (s *TokensUseCaseTestSuite) TestGetPrices_Chain_FindUnsupportedTokens() {
	// env := os.Getenv("CI_SQS_PRICING_TEST")
	// if env != "true" {
	// 	s.T().Skip("This test exists to identify which mainnet tokens are unsupported")
	// }

	s.T().Parallel()

	viper.SetConfigFile("../../config.json")
	err := viper.ReadInConfig()
	s.Require().NoError(err)

	// Unmarshal the config into your Config struct
	var config domain.Config
	err = viper.Unmarshal(&config)
	s.Require().NoError(err)

	config.Pricing.MinOSMOLiquidity = 0
	// We also set the same value on the router so that the pools are not excluded during sorting.
	config.Router.MinOSMOLiquidity = config.Pricing.MinOSMOLiquidity

	// Set up mainnet mock state.

	router, mainnetState := s.SetupMainnetRouter(*config.Router, *config.Pricing)

	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata(context.Background())
	s.Require().NoError(err)

	errorCounter := 0
	zeroPriceCounter := 0
	s.Require().NotZero(len(tokenMetadata))
	for chainDenom, tokenMeta := range tokenMetadata {
		// System under test.
		price, err := mainnetUsecase.Tokens.GetPrices(context.Background(), []string{chainDenom}, []string{USDC})
		if err != nil {
			fmt.Printf("Error for %s  -- %s\n", chainDenom, tokenMeta.HumanDenom)
			errorCounter++
			continue
		}

		priceValue, ok := price[chainDenom][USDC]
		s.Require().True(ok)

		priceBigDec := s.ConvertAnyToBigDec(priceValue)

		if priceBigDec.IsZero() {
			fmt.Printf("Zero price for %s  -- %s\n", chainDenom, tokenMeta.HumanDenom)
			zeroPriceCounter++
			continue
		}
	}

	s.Require().Zero(errorCounter)
	s.Require().Zero(zeroPriceCounter)
}

// Convinience test to test and print a result for a specific token
func (s *TokensUseCaseTestSuite) TestGetPrices_Chain_Specific() {
	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(defaultPricingRouterConfig, defaultPricingConfig)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	// System under test.
	price, err := mainnetUsecase.Tokens.GetPrices(context.Background(), []string{CRE}, []string{USDC})
	s.Require().NoError(err)

	fmt.Println(price)
}
