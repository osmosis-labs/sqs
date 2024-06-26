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

	mainnetAssetListFileURL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/generated/frontend/assetlist.json"
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
	STEVMOS = routertesting.STEVMOS
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
		CoingeckoUrl:           "https://prices.osmosis.zone/api/v3/simple/price",
		CoingeckoQuoteCurrency: "usd",
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
	s.Require().NotEmpty(atomToken.CoingeckoID)

	// ION is present
	ionMainnetDenom := "uion"
	ionToken, ok := tokensMap[ionMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, ionToken.Precision)
	s.Require().NotEmpty(ionToken.CoingeckoID)

	// IBCX is present
	ibcxMainnetDenom := "factory/osmo14klwqgkmackvx2tqa0trtg69dmy0nrg4ntq4gjgw2za4734r5seqjqm4gm/uibcx"
	ibcxToken, ok := tokensMap[ibcxMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, ibcxToken.Precision)
	s.Require().NotEmpty(ibcxToken.CoingeckoID)

	// DYSON is present, but doesn't have coingecko id
	dysonMainnetDenom := "ibc/E27CD305D33F150369AB526AEB6646A76EC3FFB1A6CA58A663B5DE657A89D55D"
	dysonToken, ok := tokensMap[dysonMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(0, dysonToken.Precision)

	// ETH is present
	ethToken, ok := tokensMap[ETH]
	s.Require().True(ok)
	s.Require().Equal(ethExponent, ethToken.Precision)
	s.Require().False(ethToken.IsUnlisted)
	s.Require().NotEmpty(ethToken.CoingeckoID)

	// AAVE is present but is unlisted
	aaveToken, ok := tokensMap[AAVE_UNLISTED]
	s.Require().True(ok)
	s.Require().True(aaveToken.IsUnlisted)
	s.Require().NotEmpty(aaveToken.CoingeckoID)
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

// This test takes some mainnet denoms (routertesting.MainnetDenoms) and fetch their prices with USDC as a quote from Coingecko API endpoint.
// It then validates that every denom has non-zero price quote as returned from Coingecko
func (s *TokensUseCaseTestSuite) TestGetPrices_Coingecko() {
	// Set up mainnet mock state.
	mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()
	prices, err := mainnetUsecase.Tokens.GetPrices(context.Background(), routertesting.MainnetDenoms, []string{USDC}, domain.CoinGeckoPricingSourceType)
	s.Require().NoError(err)
	s.Require().Len(prices, len(routertesting.MainnetDenoms))
	for _, baseAssetPrices := range prices {
		s.Require().Len(baseAssetPrices, 1)
		usdcQuoteAny, ok := baseAssetPrices[USDC]
		s.Require().True(ok)
		usdcQuote := s.ConvertAnyToBigDec(usdcQuoteAny)
		s.Require().NotZero(usdcQuote)
	}
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
	price, err := mainnetUsecase.Tokens.GetPrices(context.Background(), []string{STEVMOS}, []string{USDC}, domain.ChainPricingSourceType, domain.WithRecomputePrices(), domain.WithMinPricingPoolLiquidityCap(1))
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

// Basic sanity check test case to validate the updates and retrieval of pool denom liquidity.
// It sets up mainnet mock state and updates the pool denom metadata for ATOM and OSMO.
// It then retrieves the liquidity of ATOM and OSMO and validates if the liquidity is updated.
// It also validates if the liquidity of another token is not present.
// It then updates the OSMO liquidity and validates if the ATOM liquidity is still the same and OSMO liquidity is updated.
// Additionally, it valides that for the getter with multiple chain denoms, if the requested chain denom metadata is not present, it is nullified without erroring.
// it will be nullified without error.
func (s *TokensUseCaseTestSuite) TestPoolDenomMetadata() {

	var (
		xAmount       = osmomath.NewInt(1000)
		doubleXAmount = xAmount.Add(xAmount)
	)

	// Set up mainnet mock state.
	mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()

	// System under test.
	// Get the liquidity of ATOM
	xAmount, err := mainnetUsecase.Tokens.GetPoolLiquidityCap(ATOM)
	s.Require().Error(err)

	s.Require().ErrorIs(err, domain.PoolDenomMetaDataNotPresentError{
		ChainDenom: ATOM,
	})

	s.Require().Equal(osmomath.Int{}, xAmount)

	// Update the pool denom metadata for ATOM and OSMO
	atomPoolDenomMetadata := domain.PoolDenomMetaData{
		TotalLiquidity: xAmount,
	}

	osmoPoolDenomMetadata := domain.PoolDenomMetaData{
		TotalLiquidity: doubleXAmount,
	}

	mainnetUsecase.Tokens.UpdatePoolDenomMetadata(domain.PoolDenomMetaDataMap{
		ATOM:  atomPoolDenomMetadata,
		UOSMO: osmoPoolDenomMetadata,
	})

	// Get the liquidity of ATOM again
	atomLiquidityUpdated, err := mainnetUsecase.Tokens.GetPoolLiquidityCap(ATOM)
	s.Require().NoError(err)

	// Check if the liquidity is updated.
	s.Require().Equal(atomPoolDenomMetadata.TotalLiquidity.String(), atomLiquidityUpdated.String())

	// Get the liquidity of OSMO
	osmoLiquidityUpdated, err := mainnetUsecase.Tokens.GetPoolLiquidityCap(UOSMO)
	s.Require().NoError(err)

	// Check if the liquidity is updated.
	s.Require().Equal(osmoPoolDenomMetadata.TotalLiquidity.String(), osmoLiquidityUpdated.String())

	// Fail to get the liquidity of another token
	_, err = mainnetUsecase.Tokens.GetPoolLiquidityCap(UION)
	s.Require().Error(err)
	s.Require().ErrorIs(err, domain.PoolDenomMetaDataNotPresentError{
		ChainDenom: UION,
	})

	// Now, update only the OSMO liquidity
	osmoPoolDenomMetadataUpdated := domain.PoolDenomMetaData{
		TotalLiquidity: xAmount,
	}
	mainnetUsecase.Tokens.UpdatePoolDenomMetadata(domain.PoolDenomMetaDataMap{
		UOSMO: osmoPoolDenomMetadataUpdated,
	})

	// Get all the pool denom metadata
	poolDenomMetadata := mainnetUsecase.Tokens.GetPoolDenomsMetadata([]string{ATOM, UOSMO, UION})
	s.Require().Len(poolDenomMetadata, 3)
	for chainDenom, metadata := range poolDenomMetadata {
		switch chainDenom {
		case ATOM:
			// Validate ATOM is still the same despite only OSMO being updated
			s.Require().Equal(atomPoolDenomMetadata, metadata)
		case UOSMO:
			// 	// Validate OSMO is updated
			s.Require().Equal(osmoPoolDenomMetadataUpdated, metadata)
		case UION:
			// Validate UION is not present and is nullified without erroring.
			s.Require().Equal(osmomath.ZeroInt().String(), metadata.TotalLiquidity.String())
		}
	}
}

// Test to validate the min pool liquidity cap retrieval works as expected.
func (s *TokensUseCaseTestSuite) TestGetMinPoolLiquidityCap() {
	const (
		minLiquidityCap = 10000
		maxUint64Value  = ^uint64(0)
	)

	var (
		denomNoMetadata                = UION
		denomOverFlowA                 = USDC
		denomOverFlowB                 = USDT
		overflowVlaue                  = osmomath.NewIntFromUint64(maxUint64Value).Add(osmomath.OneInt())
		defaultPoolDenomMetadataPreSet = domain.PoolDenomMetaDataMap{
			ATOM: domain.PoolDenomMetaData{
				TotalLiquidityCap: osmomath.NewInt(minLiquidityCap),
			},
			UOSMO: domain.PoolDenomMetaData{
				TotalLiquidityCap: osmomath.NewInt(2 * minLiquidityCap),
			},
			denomOverFlowA: domain.PoolDenomMetaData{
				TotalLiquidityCap: overflowVlaue,
			},
			denomOverFlowB: domain.PoolDenomMetaData{
				TotalLiquidityCap: overflowVlaue,
			},
		}
	)

	tests := []struct {
		name string

		preSetPoolDenomMetadata domain.PoolDenomMetaDataMap

		denomA string
		denomB string

		expectedMinPoolLiquidityCap uint64
		expectError                 bool
	}{
		{
			name: "valid case",

			preSetPoolDenomMetadata: defaultPoolDenomMetadataPreSet,

			denomA: ATOM,
			denomB: UOSMO,

			expectedMinPoolLiquidityCap: minLiquidityCap,
		},
		{
			name: "denom A does not have metadata",

			preSetPoolDenomMetadata: defaultPoolDenomMetadataPreSet,

			denomA: denomNoMetadata,
			denomB: UOSMO,

			expectError: true,
		},
		{
			name: "denom B does not have metadata",

			preSetPoolDenomMetadata: defaultPoolDenomMetadataPreSet,

			denomA: ATOM,
			denomB: denomNoMetadata,

			expectError: true,
		},

		{
			name: "overflow occurs",

			preSetPoolDenomMetadata: defaultPoolDenomMetadataPreSet,

			denomA: denomOverFlowA,
			denomB: denomOverFlowB,

			expectError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		s.Run(tt.name, func() {
			// Set up mainnet mock state.
			mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()

			// System under test
			mainnetUsecase.Tokens.UpdatePoolDenomMetadata(tt.preSetPoolDenomMetadata)

			// System under test.
			actualMinPoolLiquidityCap, err := mainnetUsecase.Tokens.GetMinPoolLiquidityCap(tt.denomA, tt.denomB)

			if tt.expectError {
				s.Require().Error(err)
				return
			}

			s.Require().NoError(err)

			// Check if the min pool liquidity cap is as expected.
			s.Require().Equal(tt.expectedMinPoolLiquidityCap, actualMinPoolLiquidityCap)
		})
	}

}
