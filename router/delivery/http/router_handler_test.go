package http_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/delivery/http"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

type RouterHandlerSuite struct {
	routertesting.RouterTestHelper
}

var (
	UOSMO = routertesting.UOSMO
	USDC  = routertesting.USDC
	UATOM = routertesting.ATOM
)

func TestRouterHandlerSuite(t *testing.T) {
	suite.Run(t, new(RouterHandlerSuite))
}

func (s *RouterHandlerSuite) TestGetMinPoolLiquidityCapFilter() {

	const (
		dynamicFilterValue = 10_000
		defaultFilterValue = 100

		minTokensCapThreshold = 5_000
	)

	routerConfig := routertesting.DefaultRouterConfig
	routerConfig.DynamicMinLiquidityCapFiltersDesc = []domain.DynamicMinLiquidityCapFilterEntry{
		{
			MinTokensCap: 5_000,
			// This is what should be returned with dynamic min liquidity enabled
			// for UOSMO and USDC since both of these tokens have way more mainnet liquidity
			//	than $5K
			FilterValue: dynamicFilterValue,
		},
	}
	// Universal default min liquidity cap is $100
	// This is what's returned if we fallback by default.
	routerConfig.MinPoolLiquidityCap = defaultFilterValue

	tests := []struct {
		name string

		tokenInDenom                string
		tokenOutDenom               string
		disableMinLiquidityFallback bool

		expectedFilter uint64
		expectErr      bool
	}{
		{
			name:           "min liquidity fallback is enabled",
			tokenInDenom:   USDC,
			tokenOutDenom:  UOSMO,
			expectedFilter: dynamicFilterValue,
		},
		{
			name:                        "min liquidity fallback is disabled but no error",
			tokenInDenom:                USDC,
			tokenOutDenom:               UOSMO,
			disableMinLiquidityFallback: true,
			expectedFilter:              dynamicFilterValue,
		},
		{
			name: "error due to token with no metadata and fallback enabled",
			// UATOM does not have the pool liquidity metadata pre-configured.
			tokenInDenom:   UATOM,
			tokenOutDenom:  UOSMO,
			expectedFilter: defaultFilterValue,
		},
		{
			name: "error due to token with no metadata and fallback enabled",
			// UATOM does not have the pool liquidity metadata pre-configured.
			tokenInDenom:                UATOM,
			tokenOutDenom:               UOSMO,
			disableMinLiquidityFallback: true,

			expectErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc

		s.T().Run(tc.name, func(t *testing.T) {
			// Set up mainnet mock state.
			mainnetState := s.SetupMainnetState()

			mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(routerConfig), routertesting.WithLoggerDisabled())

			routerHandler := http.RouterHandler{
				RUsecase: mainnetUsecase.Router,
				TUsecase: mainnetUsecase.Tokens,
			}

			mainnetUsecase.Tokens.UpdatePoolDenomMetadata(domain.PoolDenomMetaDataMap{
				USDC: domain.PoolDenomMetaData{
					TotalLiquidityCap: osmomath.NewInt(minTokensCapThreshold + 1),
				},
				UOSMO: domain.PoolDenomMetaData{
					TotalLiquidityCap: osmomath.NewInt(minTokensCapThreshold + 1),
				},
			})

			actualFilter, err := routerHandler.GetMinPoolLiquidityCapFilter(tc.tokenInDenom, tc.tokenOutDenom, tc.disableMinLiquidityFallback)

			if tc.expectErr {
				s.Require().Error(err)
				return
			}

			// Validate result
			s.Require().NoError(err)
			s.Require().Equal(tc.expectedFilter, actualFilter)
		})
	}
}
