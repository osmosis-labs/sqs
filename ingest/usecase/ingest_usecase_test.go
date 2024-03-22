package usecase_test

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

type IngesterTestSuite struct {
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
		MinOSMOLiquidity:       50,
	}
)

func TestIngesterTestSuite(t *testing.T) {
	suite.Run(t, new(IngesterTestSuite))
}

// TODO: improve test
func (s *IngesterTestSuite) TestComputeCoinTVL() {
	s.T().Skip("skipping test")

	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(defaultPricingRouterConfig, defaultPricingConfig)
	mainnetState.PricingConfig = defaultPricingConfig
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	tokenMetadata, err := mainnetUsecase.Tokens.GetFullTokenMetadata(s.Ctx)
	s.Require().NoError(err)

	ingestUsecaseImpl, ok := mainnetUsecase.Ingest.(*usecase.IngestUseCaseImpl)
	s.Require().True(ok)

	s.Require().NotZero(len(tokenMetadata))
	for chainDenom, token := range tokenMetadata {
		chainAmount := osmomath.NewDec(10).PowerMut(uint64(token.Precision)).TruncateInt()
		_, err := ingestUsecaseImpl.ComputeCoinTVL(context.Background(), sdk.NewCoin(chainDenom, chainAmount))

		if err != nil {
			fmt.Println(err)
		}
	}
}
