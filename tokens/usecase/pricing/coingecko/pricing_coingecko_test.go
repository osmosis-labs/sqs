package coingeckopricing_test

import (
	"context"
	"fmt"
	"os"
	"testing"

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
// The main goal of this test is to check if the GetPrice method returns 0 and error given non-supported base or quote denom
func (s *CoingeckoPricingTestSuite) TestGetPrices() {

	mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()
	defaultPricingConfig.DefaultSource = domain.CoinGeckoPricingSourceType
	coingeckoPricingSource := coingeckopricing.New(mainnetUsecase.Router, mainnetUsecase.Tokens, defaultPricingConfig, mocks.DefaultMockCoingeckoPriceGetter)

	tests := []struct {
		desc       string
		baseDenom  string
		quoteDenom string
		shouldZero bool
		shouldErr  bool
	}{
		{"Test coingecko GetPrice with quote denom as USDC", ATOM, USDC, false, false},
		{"Test coingecko GetPrice with quote denom as USDT", ATOM, USDT, false, false},
		{"Test coingecko GetPrice with quote denom as empty string", ATOM, "", false, false},
		{"Test coingecko GetPrice with quote denom as some spaces", ATOM, " ", false, false},
		{"Test coingecko GetPrice with quote denom as ATOM", ATOM, ETH, true, true},
		{"Test coingecko GetPrice with invalid base denom", "-DUMMY-", USDC, true, true},
		{"Test coingecko GetPrice with empty base denom", "", USDC, true, true},
		{"Test coingecko GetPrice with some spaces as base denom", " ", USDC, true, true},
	}

	for _, tt := range tests {
		s.Run(tt.desc, func() {
			price, err := coingeckoPricingSource.GetPrice(context.Background(), tt.baseDenom, tt.quoteDenom)
			if tt.shouldErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
			if tt.shouldZero {
				s.Require().Zero(price)
			} else {
				s.Require().NotZero(price)
			}
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
