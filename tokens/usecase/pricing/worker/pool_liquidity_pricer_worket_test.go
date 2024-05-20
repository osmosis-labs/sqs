package worker_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

type PoolLiquidityComputeWorkerSuite struct {
	routertesting.RouterTestHelper
}

var (
	defaultPricingRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:    []uint64{},
		MaxRoutes:           5,
		MaxPoolsPerRoute:    3,
		MaxSplitRoutes:      3,
		MinPoolLiquidityCap: 50,
		RouteCacheEnabled:   true,
	}

	pricingCacheExpiry = 2000
)

var (
	stableCoinDenoms = []string{"usdc", "usdt", "dai", "ist"}
)

func TestPoolLiquidityComputeWorkerSuite(t *testing.T) {
	suite.Run(t, new(PoolLiquidityComputeWorkerSuite))
}

func (s *PoolLiquidityComputeWorkerSuite) TestOnPricingUpdate() {

	tests := []struct {
		name string
	}{}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

		})
	}
}
