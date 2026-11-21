package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
	"github.com/stretchr/testify/suite"
)

type PricingWorkerTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultHeight uint64 = 1
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
			name:       "empty base denoms",
			baseDenoms: domain.BlockPoolMetadata{},
		},
		{
			name: "one base denom",
			baseDenoms: domain.BlockPoolMetadata{
				UpdatedDenoms: map[string]struct{}{
					UOSMO: {},
				},
			},
		},
		{
			name: "several base denoms",
			baseDenoms: domain.BlockPoolMetadata{
				UpdatedDenoms: map[string]struct{}{
					UOSMO: {},
					ATOM:  {},
					USDC:  {},
				},
			},
		},
		{
			name: "several base denoms with a queued base denom",
			baseDenoms: domain.BlockPoolMetadata{
				UpdatedDenoms: map[string]struct{}{
					UOSMO: {},
					USDC:  {},
				},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			mainnetUsecase := s.SetupDefaultRouterAndPoolsUsecase()

			defaultQuoteDenom, err := mainnetUsecase.Tokens.GetChainDenom(defaultPricingConfig.DefaultQuoteHumanDenom)
			s.Require().NoError(err)

			// Create a pricing worker
			pricingWorker := worker.New(mainnetUsecase.Tokens, defaultQuoteDenom, defaultPricingConfig.WorkerMinPoolLiquidityCap, &log.NoOpLogger{})

			// Create a mock listener
			mockPricingUpdateListener := mocks.NewPricingListenerMock(time.Second * 5)
			mockPricingUpdateListener.QuoteDenom = defaultQuoteDenom

			// Register the listener
			pricingWorker.RegisterListener(mockPricingUpdateListener)

			pricingWorker.UpdatePricesAsync(context.Background(), defaultHeight, tc.baseDenoms)

			// Wait for the listener to be called. The listener's fields are read only
			// after it signals: before that, the background run may be writing them.
			didTimeout := mockPricingUpdateListener.WaitOrTimeout()
			s.Require().False(didTimeout)

			// Ensure that the correct number of base denoms are set
			s.Require().Equal(len(tc.baseDenoms.UpdatedDenoms), len(mockPricingUpdateListener.PricesBaseQuteDenomMap))

			// Ensure that non-zero prices are set for each base denom
			s.ValidatePrices(tc.baseDenoms.UpdatedDenoms, defaultQuoteDenom, mockPricingUpdateListener.PricesBaseQuteDenomMap)
		})
	}
}

func (s *PricingWorkerTestSuite) ValidatePrices(initialDenoms map[string]struct{}, expectedQuoteDenom string, prices map[string]map[string]osmomath.BigDec) {
	for baseDenom := range initialDenoms {
		quoteMap, ok := prices[baseDenom]
		s.Require().True(ok)

		price, ok := quoteMap[expectedQuoteDenom]
		s.Require().True(ok)

		s.Require().NotZero(price)
	}
}
