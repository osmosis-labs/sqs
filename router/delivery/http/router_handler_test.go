package http_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	routerdelivery "github.com/osmosis-labs/sqs/router/delivery/http"
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
		forceDefaultMinLiquidityCap bool

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

		{
			name: "forcing default min liqudity cap prevents error",
			// UATOM does not have the pool liquidity metadata pre-configured.
			tokenInDenom:                UATOM,
			tokenOutDenom:               UOSMO,
			forceDefaultMinLiquidityCap: true,

			expectedFilter: defaultFilterValue,
		},
	}

	for _, tc := range tests {
		tc := tc

		s.T().Run(tc.name, func(t *testing.T) {
			// Set up mainnet mock state.
			mainnetState := s.SetupMainnetState()

			mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(routerConfig), routertesting.WithLoggerDisabled())

			routerHandler := routerdelivery.RouterHandler{
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

			actualFilter, err := routerHandler.GetMinPoolLiquidityCapFilter(tc.tokenInDenom, tc.tokenOutDenom, tc.disableMinLiquidityFallback, tc.forceDefaultMinLiquidityCap)

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

// TestGetPoolsValidTokenInTokensOut tests parsing pools, token in and token out parameters
// from the request.
func TestGetPoolsValidTokenInTokensOut(t *testing.T) {
	testCases := []struct {
		name string

		// input
		uri string

		// expected output
		tokenIn  string
		poolIDs  []uint64
		tokenOut []string

		err error
	}{
		{
			name:     "happy case - token through single pool",
			uri:      "http://localhost?tokenIn=10OSMO&poolID=1&tokenOutDenom=USDC",
			tokenIn:  "10OSMO",
			poolIDs:  []uint64{1},
			tokenOut: []string{"USDC"},
		},
		{
			name: "fail case - token through single pool",
			uri:  "http://localhost?tokenIn=&poolID=1&tokenOutDenom=USDC",
			err:  routerdelivery.ErrTokenNotSpecified,
		},
		{
			name:     "happy case - token through multi pool",
			uri:      "http://localhost?tokenIn=56OSMO&poolID=1,5,7&tokenOutDenom=ATOM,AKT,USDC",
			tokenIn:  "56OSMO",
			poolIDs:  []uint64{1, 5, 7},
			tokenOut: []string{"ATOM", "AKT", "USDC"},
		},
		{
			name: "fail case - token through multi pool",
			uri:  "http://localhost?tokenIn=56OSMO&poolID=1,5&tokenOutDenom=ATOM,AKT,USDC",
			err:  routerdelivery.ErrNumOfTokenOutDenomPoolsMismatch,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := echo.New().NewContext(
				httptest.NewRequest(http.MethodGet, tc.uri, nil),
				nil,
			)

			poolIDs, tokenOut, tokenIn, err := routerdelivery.GetPoolsValidTokenInTokensOut(ctx)
			if !errors.Is(err, tc.err) {
				t.Fatalf("got %v, want %v", err, tc.err)
			}

			// on error output of the function is undefined
			if err != nil {
				t.SkipNow()
			}

			if slices.Compare(poolIDs, tc.poolIDs) != 0 {
				t.Fatalf("got %v, want %v", poolIDs, tc.poolIDs)
			}

			if slices.Compare(tokenOut, tc.tokenOut) != 0 {
				t.Fatalf("got %v, want %v", tokenOut, tc.tokenOut)
			}

			if tokenIn != tc.tokenIn {
				t.Fatalf("got %v, want %v", tokenIn, tc.tokenIn)
			}
		})
	}
}
