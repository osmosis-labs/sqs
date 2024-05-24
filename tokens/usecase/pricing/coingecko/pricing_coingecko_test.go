package coingeckopricing_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	coingeckopricing "github.com/osmosis-labs/sqs/tokens/usecase/pricing/coingecko"
	"github.com/stretchr/testify/suite"
)

type CoingeckoPricingTestSuite struct {
	routertesting.RouterTestHelper
}

var (
	USDC                 = routertesting.USDC
	USDT                 = routertesting.USDT
	ATOM                 = routertesting.ATOM
	ETH                  = routertesting.ETH
	defaultRouterConfig  = routertesting.DefaultRouterConfig
	defaultPricingConfig = routertesting.DefaultPricingConfig
)

// Set up mainnet mock state with default router and pools usecase.
func (s *CoingeckoPricingTestSuite) SetupDefaultRouterAndPoolsUsecase() routertesting.MockMainnetUsecase {
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))
	return mainnetUsecase
}

func TestCoingeckoPricingTestSuite(t *testing.T) {
	suite.Run(t, new(CoingeckoPricingTestSuite))
}

// TestGetPrices tests the GetPrice method of CoingeckoPricing.
// MockCoingeckoPriceGetter is used to mock the CoingeckoPriceGetterFn function
// The main goal of this test is to check if the GetPrice method returns 0 given non-supported base or quote denom
// and if it returns the expected price given supported base and quote denom.
func (s *CoingeckoPricingTestSuite) TestGetPrices() {

	mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()
	defaultPricingConfig.DefaultSource = domain.CoinGeckoPricingSourceType
	coingeckoPricingSource := coingeckopricing.New(mainnetUsecase.Tokens, defaultPricingConfig, mocks.DefaultMockCoingeckoPriceGetter)

	tests := []struct {
		desc          string
		baseDenom     string
		quoteDenom    string
		expectedPrice osmomath.BigDec
		shouldErr     bool
	}{
		{"Test coingecko GetPrice with quote denom as USDC", ATOM, USDC, mocks.AtomPrice, false},
		{"Test coingecko GetPrice with quote denom as USDT", ATOM, USDT, mocks.AtomPrice, false},
		{"Test coingecko GetPrice with quote denom as empty string", ATOM, "", mocks.AtomPrice, false},
		{"Test coingecko GetPrice with quote denom as some spaces", ATOM, " ", mocks.AtomPrice, false},
		{"Test coingecko GetPrice with quote denom as USDC", ETH, USDC, mocks.OneBigDec, false},
		{"Test coingecko GetPrice with quote denom as USDT", ETH, USDT, mocks.OneBigDec, false},
		{"Test coingecko GetPrice with quote denom as empty string", ETH, "", mocks.OneBigDec, false},
		{"Test coingecko GetPrice with quote denom as some spaces", ETH, " ", mocks.OneBigDec, false},
		{"Test coingecko GetPrice with quote denom as ATOM", ATOM, ETH, mocks.ZeroBigDec, true},
		{"Test coingecko GetPrice with invalid base denom", "-DUMMY-", USDC, mocks.ZeroBigDec, true},
		{"Test coingecko GetPrice with empty base denom", "", USDC, mocks.ZeroBigDec, true},
		{"Test coingecko GetPrice with some spaces as base denom", " ", USDC, mocks.ZeroBigDec, true},
	}

	for _, tt := range tests {
		s.Run(tt.desc, func() {
			price, err := coingeckoPricingSource.GetPrice(context.Background(), tt.baseDenom, tt.quoteDenom)
			s.Require().Equal(tt.expectedPrice, price)
			s.Require().Equal(tt.shouldErr, err != nil)
		})
	}

}

// TestGetPrices_Coingecko_FindUnsupportedTokens is a test to identify which mainnet tokens are unsupported tokens in Coingecko.
func (s *CoingeckoPricingTestSuite) TestGetPrices_Coingecko_FindUnsupportedTokens() {
	env := os.Getenv("CI_SQS_PRICING_COINGECKO_TEST")
	if env != "true" {
		s.T().Skip("This test exists to identify which mainnet tokens are unsupported tokens in Coingecko")
	}

	// Set up mainnet mock state.
	mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()

	// Get all token metadata.
	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata()
	s.Require().NoError(err)
	s.Require().NotZero(len(tokenMetadata))

	unsupportedCounter := 0
	for chainDenom := range tokenMetadata {
		coingeckoId, err := mainnetUsecase.Tokens.GetCoingeckoIdByChainDenom(chainDenom)
		if err != nil {
			fmt.Println("Error in getting coingecko id for chain denom: ", chainDenom)
		} else if coingeckoId == "" {
			fmt.Println("Unsupported token: ", chainDenom)
			unsupportedCounter++
		} else {
			fmt.Println("Supported token: ", chainDenom, " with coingecko id: ", coingeckoId)
		}
	}

	fmt.Println("Total unsupported tokens: ", unsupportedCounter)

	// Total unsupported tokens as of May 13 2024: 137
	s.Require().Equal(137, unsupportedCounter)

}
