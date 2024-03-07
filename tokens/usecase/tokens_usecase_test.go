package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
)

type TokensUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultCosmosExponent     = 6
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
		DefaultSource:          domain.ChainPricingSource,
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

func (s *TokensUseCaseTestSuite) TestParseExponents() {
	s.T().Skip("skip the test that does network call and is used for debugging")

	const ()
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

	// IBCX is presnet
	ibcxMainnetDenom := "factory/osmo14klwqgkmackvx2tqa0trtg69dmy0nrg4ntq4gjgw2za4734r5seqjqm4gm/uibcx"
	ibcxToken, ok := tokensMap[ibcxMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, ibcxToken.Precision)
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
	router, mainnetState := s.SetupMainnetRouter(defaultPricingRouterConfig)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	// Set up on-chain pricing strategy
	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	// System under test.
	prices, err := mainnetUsecase.Tokens.GetPrices(context.Background(), routertesting.MainnetDenoms, []string{USDC, USDT}, pricingStrategy)
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
		usdcQuote := s.convertAnyToBigDec(usdcQuoteAny)

		usdtQuoteAny, ok := baseAssetPrices[USDT]
		s.Require().True(ok)
		usdtQuote := s.convertAnyToBigDec(usdtQuoteAny)

		result := errTolerance.CompareBigDec(usdcQuote, usdtQuote)
		s.Require().Zero(result)
	}

	// WBTC is around 50K at the time of creation of this test
	// We set tolerance to 15% and compare against this value to have sanity checks
	// in place against a hardcoded expected value rather than comparing USDT and USDC prices only
	// that are both computed by the system.
	// Noe: if WBTC price changes by more than 15% and we update test mainnet state, this test is likely to fail.
	expectedwBTCPrice := osmomath.NewBigDec(50000)
	wbtcErrorTolerance := osmomath.ErrTolerance{
		// 15% tolerance
		MultiplicativeTolerance: osmomath.MustNewDecFromStr("0.15"),
	}

	actualwBTCUSDCPriceAny, ok := prices[WBTC][USDC]
	s.Require().True(ok)
	actualwBTCUSDCPrice := s.convertAnyToBigDec(actualwBTCUSDCPriceAny)

	result := wbtcErrorTolerance.CompareBigDec(expectedwBTCPrice, actualwBTCUSDCPrice)
	s.Require().Zero(result)
}

func (s *TokensUseCaseTestSuite) TestGetPrices_Chain_FindUnsupportedTokens() {
	s.T().Skip("This test exists to identify which mainnet tokens are unsupported")

	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(defaultPricingRouterConfig)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata(context.Background())
	s.Require().NoError(err)

	// Set up on-chain pricing strategy
	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	counter := 1
	for chainDenom, tokenMetadata := range tokenMetadata {
		// System under test.
		_, err = mainnetUsecase.Tokens.GetPrices(context.Background(), []string{chainDenom}, []string{USDC}, pricingStrategy)
		if err != nil {
			fmt.Printf("%d. %s\n", counter, tokenMetadata.HumanDenom)
			counter++
		}
	}
}

// Convinience test to test and print a result for a specific token
func (s *TokensUseCaseTestSuite) TestGetPrices_Chain_Specific() {
	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(defaultPricingRouterConfig)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	// Set up on-chain pricing strategy
	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	// System under test.
	price, err := mainnetUsecase.Tokens.GetPrices(context.Background(), []string{CRE}, []string{USDC}, pricingStrategy)
	s.Require().NoError(err)

	fmt.Println(price)
}

// helper to convert any to BigDec
func (s *TokensUseCaseTestSuite) convertAnyToBigDec(any any) osmomath.BigDec {
	bigDec, ok := any.(osmomath.BigDec)
	s.Require().True(ok)
	return bigDec
}
