package usecase_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

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
	// As of 2024-05, this token is unlisted but this might change.
	AAVE_UNLISTED = "ibc/384E5DD50BDE042E1AAF51F312B55F08F95BC985C503880189258B4D9374CBBE"

	defaultPricingRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:    []uint64{},
		MaxRoutes:           5,
		MaxPoolsPerRoute:    3,
		MaxSplitRoutes:      3,
		MinPoolLiquidityCap: 50,
		RouteCacheEnabled:   true,
	}

	defaultPricingConfig = domain.PricingConfig{
		DefaultSource:          domain.ChainPricingSourceType,
		CacheExpiryMs:          pricingCacheExpiry,
		DefaultQuoteHumanDenom: "usdc",
		MaxPoolsPerRoute:       4,
		MaxRoutes:              5,
		MinPoolLiquidityCap:    50,
	}
)

func TestTokensUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(TokensUseCaseTestSuite))
}

func (s *TokensUseCaseTestSuite) SetupDefaultRouterAndPoolsUsecase() routertesting.MockMainnetUsecase {
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))
	return mainnetUsecase
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
	s.Require().False(ethToken.IsUnlisted)

	// AAVE is present but is unlisted
	aaveToken, ok := tokensMap[AAVE_UNLISTED]
	s.Require().True(ok)
	s.Require().True(aaveToken.IsUnlisted)
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
// Additionally, for sanity check it confirms that for WBTC / USDC the price is within 15% of 50K
// (approximately the real price at the time of writing)
func (s *TokensUseCaseTestSuite) TestGetPrices_Chain() {

	// Set up mainnet mock state.
	mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()

	// System under test.
	prices, err := mainnetUsecase.Tokens.GetPrices(context.Background(), routertesting.MainnetDenoms, []string{USDC, USDT}, domain.ChainPricingSourceType)
	s.Require().NoError(err)

	errTolerance := osmomath.ErrTolerance{
		// 6% tolerance
		MultiplicativeTolerance: osmomath.MustNewDecFromStr("0.06"),
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

// Convinience test to test and print a result for a specific token
func (s *TokensUseCaseTestSuite) TestGetPrices_Chain_Specific() {
	// Set up mainnet mock state.
	mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()

	// System under test.
	price, err := mainnetUsecase.Tokens.GetPrices(context.Background(), []string{CRE}, []string{USDC}, domain.ChainPricingSourceType)
	s.Require().NoError(err)

	fmt.Println(price)
}

// Test to validate the pricing options work as expected.
// Currently, only tests recompute pricing options. In the future, we also add pricing options for the source,
// once more sources are supported.
func (s *TokensUseCaseTestSuite) TestGetPrices_Chain_PricingOptions() {

	var (
		defaultBase  = ATOM
		defaultQuote = USDC

		defaultBaseInput, defaultQuoteInput = []string{defaultBase}, []string{defaultQuote}

		// We are hoping that the price of ATOM only goes up and never reaches one.
		// As a result, it is reasonable to assume that in tests and use it as a cache overwrite for testing.
		priceOne = osmomath.OneBigDec()

		// // placeholder to reflect no value in cache
		// unsetCachedPrice = osmomath.BigDec{}

		// // Empty pricing options imply no recompute and chain source.
		// emptyPricingOptions = []domain.PricingOption{}
	)

	// Compute the mainnet price with no cache set (empty)
	// Note that this is naive because we use the system under test for configuring the test.
	// However, the likelyhood of this causing errors is low if other GetPrices tests are passing.
	// If there is confusion with this test, first make
	// sure that other GetPrices tests are OK. Then, come back to this.

	// Set up mainnet mock state.
	mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()

	noCacheMainnetPrice, err := mainnetUsecase.Tokens.GetPrices(context.Background(), defaultBaseInput, defaultQuoteInput, domain.ChainPricingSourceType, domain.WithRecomputePrices())
	s.Require().NoError(err)

	recomputedPrice := s.ConvertAnyToBigDec(noCacheMainnetPrice[defaultBase][defaultQuote])

	tests := []struct {
		name string

		pricingOptions []domain.PricingOption

		cachedPrice osmomath.BigDec

		expectedPrice osmomath.BigDec
	}{
		// {
		// 	name:           "Empty cache, with recompute -> recomputes",
		// 	cachedPrice:    unsetCachedPrice,
		// 	pricingOptions: []domain.PricingOption{domain.WithRecomputePrices()},

		// 	expectedPrice: recomputedPrice,
		// },
		// {
		// 	name:        "Empty cache, with no recompute -> still recomputes",
		// 	cachedPrice: unsetCachedPrice,

		// 	pricingOptions: emptyPricingOptions,

		// 	expectedPrice: recomputedPrice,
		// },
		{
			name:           "Non-empty cache, with recompute -> recomputes and gets a different value",
			cachedPrice:    priceOne,
			pricingOptions: []domain.PricingOption{domain.WithRecomputePrices()},

			expectedPrice: recomputedPrice,
		},
		// {
		// 	name:           "Non-empty cache, with no recompute -> gets value from cache.",
		// 	cachedPrice:    priceOne,
		// 	pricingOptions: emptyPricingOptions,

		// 	expectedPrice: priceOne,
		// },
	}

	for _, tt := range tests {
		tt := tt
		s.Run(tt.name, func() {

			// Initialize pricing cache.
			pricingCache := cache.New()

			// Pre-set cache if configured.
			if !tt.cachedPrice.IsNil() {
				baseQuoteCacheKey := domain.FormatPricingCacheKey(defaultBase, defaultQuote)
				pricingCache.Set(baseQuoteCacheKey, tt.cachedPrice, defaultPricingCacheExpiry)
			}

			// Set up mainnet mock state.
			mainnetState := s.SetupMainnetState()

			// Setup mainnet use cases
			mainnetUseCase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithPricingCache(pricingCache), routertesting.WithPricingConfig(defaultPricingConfig), routertesting.WithRouterConfig(defaultPricingRouterConfig))

			// System under test.

			priceResult, err := mainnetUseCase.Tokens.GetPrices(context.Background(), defaultBaseInput, defaultQuoteInput, domain.ChainPricingSourceType, tt.pricingOptions...)
			s.Require().NoError(err)

			baseResult, ok := priceResult[defaultBase]
			s.Require().True(ok)

			actualPrice := s.ConvertAnyToBigDec(baseResult[defaultQuote])

			// Check if the price is as expected.
			s.Require().Equal(tt.expectedPrice.String(), actualPrice.String())
		})
	}
}
