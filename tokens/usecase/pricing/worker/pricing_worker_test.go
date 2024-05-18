package worker_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
)

type PricingWorkerTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultHeight = 1
)

var (
	UOSMO = routertesting.UOSMO
	ATOM  = routertesting.ATOM
	USDC  = routertesting.USDC

	defaultRouterConfig  = routertesting.DefaultRouterConfig
	defaultPricingConfig = routertesting.DefaultPricingConfig

	// It is irrelevant to the test so we set this default universally.
	defaultTotalLiquidity = osmomath.OneInt()
)

func TestPricingWorkerTestSuite(t *testing.T) {
	suite.Run(t, new(PricingWorkerTestSuite))
}

func (s *PricingWorkerTestSuite) SetupDefaultRouterAndPoolsUsecase() routertesting.MockMainnetUsecase {
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))
	return mainnetUsecase
}

// TestUpdatePricesAsync tests the UpdatePricesAsync method.
// Tests asyncronous updating of prices for a given set of base denoms by utilzing a mock listener
// with a 5 second timeout.
func (s *PricingWorkerTestSuite) TestUpdatePricesAsync() {
	testCases := []struct {
		name       string
		baseDenoms domain.BlockPoolMetadata
	}{
		{
			name: "empty base denoms",
			baseDenoms: domain.BlockPoolMetadata{
				DenomLiquidityMap: domain.DenomLiquidityMap{},
			},
		},
		{
			name: "one base denom",
			baseDenoms: domain.BlockPoolMetadata{
				DenomLiquidityMap: domain.DenomLiquidityMap{UOSMO: {
					TotalLiquidity: osmomath.OneInt(),
				}},
			},
		},
		{
			name: "several base denoms",
			baseDenoms: domain.BlockPoolMetadata{
				DenomLiquidityMap: domain.DenomLiquidityMap{
					UOSMO: {},
					ATOM:  {},
					USDC:  {},
				},
			},
		},
		{
			name: "several base denoms with a queued base denom",
			baseDenoms: domain.BlockPoolMetadata{
				DenomLiquidityMap: domain.DenomLiquidityMap{
					UOSMO: {},
					USDC:  {},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()

			defaultQuoteDenom, err := mainnetUsecase.Tokens.GetChainDenom(defaultPricingConfig.DefaultQuoteHumanDenom)
			s.Require().NoError(err)

			// Create a pricing worker
			pricingWorker := worker.New(mainnetUsecase.Tokens, defaultQuoteDenom, &log.NoOpLogger{})

			// Create a mock listener
			mockPricingUpdateListener := mocks.NewPricingListenerMock(time.Second * 5)
			mockPricingUpdateListener.QuoteDenom = defaultQuoteDenom

			// Register the listener
			pricingWorker.RegisterListener(mockPricingUpdateListener)

			// Test for empty base denoms
			// Expect no update to be triggered
			pricingWorker.UpdatePricesAsync(defaultHeight, tc.baseDenoms)

			// Height and prices are not updated
			s.Require().Zero(mockPricingUpdateListener.Height)
			s.Require().Empty(mockPricingUpdateListener.PricesBaseQuteDenomMap)

			// Wait for the listener to be called
			didTimeout := mockPricingUpdateListener.WaitOrTimeout()
			s.Require().False(didTimeout)

			// Ensure that the correct number of base denoms are set
			s.Require().Equal(len(tc.baseDenoms.DenomLiquidityMap), len(mockPricingUpdateListener.PricesBaseQuteDenomMap))

			// Ensure that non-zero prices are set for each base denom
			s.ValidatePrices(tc.baseDenoms.DenomLiquidityMap, defaultQuoteDenom, mockPricingUpdateListener.PricesBaseQuteDenomMap)
		})
	}
}

func (s *PricingWorkerTestSuite) TestGetPrices_Chain_FindUnsupportedTokens() {
	env := os.Getenv("CI_SQS_PRICING_WORKER_TEST")
	if env != "true" {
		s.T().Skip("This test exists to identify which mainnet tokens are unsupported")
	}

	viper.SetConfigFile("../../../../config.json")
	err := viper.ReadInConfig()
	s.Require().NoError(err)

	// Unmarshal the config into your Config struct
	var config domain.Config
	err = viper.Unmarshal(&config)
	s.Require().NoError(err)

	mainnetState := s.SetupMainnetState()

	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(*config.Router), routertesting.WithPricingConfig(*config.Pricing), routertesting.WithPoolsConfig(*config.Pools))

	defaultQuoteDenom, err := mainnetUsecase.Tokens.GetChainDenom(defaultPricingConfig.DefaultQuoteHumanDenom)
	s.Require().NoError(err)

	// Create a pricing worker
	pricingWorker := worker.New(mainnetUsecase.Tokens, defaultQuoteDenom, &log.NoOpLogger{})

	// Create a mock listener
	mockPricingUpdateListener := mocks.NewPricingListenerMock(time.Minute * 5)
	mockPricingUpdateListener.QuoteDenom = defaultQuoteDenom

	// Register the listener
	pricingWorker.RegisterListener(mockPricingUpdateListener)

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata()
	s.Require().NoError(err)
	s.Require().NotZero(len(tokenMetadata))

	// Populate base denoms with all possible chain denoms
	baseDenoms := domain.BlockPoolMetadata{
		DenomLiquidityMap: domain.DenomLiquidityMap{},
	}
	for chainDenom := range tokenMetadata {
		baseDenoms.DenomLiquidityMap[chainDenom] = domain.DenomLiquidityData{}
	}

	// Test for empty base denoms
	// Expect no update to be triggered
	pricingWorker.UpdatePricesAsync(defaultHeight, baseDenoms)

	// Wait for the listener to be called
	didTimeout := mockPricingUpdateListener.WaitOrTimeout()
	s.Require().False(didTimeout)

	errorCounter := 0
	zeroPriceCounter := 0
	s.Require().NotZero(len(mockPricingUpdateListener.PricesBaseQuteDenomMap))
	for baseDenom, quotePrices := range mockPricingUpdateListener.PricesBaseQuteDenomMap {

		s.Require().NotZero(len(quotePrices))

		price, ok := quotePrices[defaultQuoteDenom]
		s.Require().True(ok)

		priceBigDec := s.ConvertAnyToBigDec(price)

		if priceBigDec.IsZero() {
			metadata, ok := mainnetState.TokensMetadata[baseDenom]
			s.Require().True(ok)

			fmt.Printf("Zero price for %s, isUnlisted: %t\n", metadata.HumanDenom, metadata.IsUnlisted)
			zeroPriceCounter++
			continue
		}
	}

	s.Require().Zero(errorCounter)

	// Measured at the time of test creation.
	// Most tokens are unlisted.
	// Out of listed:
	// 	BSKT - listed but no pools
	// ASTRO.cw20 - CW pool (unsupported in tests but should be supported on mainnet)
	// BJUNO - listed but no pools
	// HARD - listed but no pools
	// MUSE - listed but no pools0
	// NSTK - listed but no pools
	// SAIL - CW pool (unsupported in tests but should be supported on mainnet)
	// ASTRO - CW pool (unsupported in tests but should be supported on mainnet)
	// SHARK - CW pool (unsupported in tests but should be supported on mainnet)
	// FURY - listed but no pools
	// FURY.legacy - listed but no pools
	s.Require().Equal(25, zeroPriceCounter)
}

func (s *PricingWorkerTestSuite) ValidatePrices(initialDenoms domain.DenomLiquidityMap, expectedQuoteDenom string, prices map[string]map[string]osmomath.BigDec) {
	for baseDenom := range initialDenoms {
		quoteMap, ok := prices[baseDenom]
		s.Require().True(ok)

		price, ok := quoteMap[expectedQuoteDenom]
		s.Require().True(ok)

		s.Require().NotZero(price)
	}
}
