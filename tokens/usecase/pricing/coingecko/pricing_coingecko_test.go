package coingeckopricing_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

type CoingeckoPricingTestSuite struct {
	routertesting.RouterTestHelper
}

var (
	defaultRouterConfig  = routertesting.DefaultRouterConfig
	defaultPricingConfig = routertesting.DefaultPricingConfig
)

func (suite *CoingeckoPricingTestSuite) BeforeTest(suiteName, testName string) {
	// Run some code before each test
}

func (s *CoingeckoPricingTestSuite) SetupDefaultRouterAndPoolsUsecase() routertesting.MockMainnetUsecase {
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))
	return mainnetUsecase
}

func TestCoingeckoPricingTestSuite(t *testing.T) {
	suite.Run(t, new(CoingeckoPricingTestSuite))
}

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

}
