package http_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	tokensdelivery "github.com/osmosis-labs/sqs/tokens/delivery/http"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/suite"
)

type TokensHandlerSuite struct {
	suite.Suite
}

func TestTokensHandlerSuite(t *testing.T) {
	suite.Run(t, new(TokensHandlerSuite))
}

func (s *TokensHandlerSuite) TestGetPrices() {
	// Mock price data
	mockPriceResult := domain.PricesResult{
		"uosmo": {
			"usdc": domain.PriceResult{
				Price:  osmomath.MustNewBigDecFromStr("1.25"),
				Routes: []domain.SplitRoute{},
			},
		},
		"uatom": {
			"usdc": domain.PriceResult{
				Price:  osmomath.MustNewBigDecFromStr("10.50"),
				Routes: []domain.SplitRoute{},
			},
		},
	}

	// Mock price data with routes for debug mode
	mockPriceResultWithRoutes := domain.PricesResult{
		"uosmo": {
			"usdc": domain.PriceResult{
				Price: osmomath.MustNewBigDecFromStr("1.25"),
				Routes: []domain.SplitRoute{
					&mocks.RouteMock{
						GetPoolsFunc: func() []domain.RoutablePool {
							return []domain.RoutablePool{
								&mocks.MockRoutablePool{
									ID: 1,
								},
							}
						},
						GetAmountOutFunc: func() osmomath.Int { return osmomath.NewInt(1000) },
						GetAmountInFunc:  func() osmomath.Int { return osmomath.NewInt(800) },
					},
				},
			},
		},
	}

	testcases := []struct {
		name               string
		queryParams        map[string]string
		handler            *tokensdelivery.TokensHandler
		expectedStatusCode int
		expectedResponse   string
		expectedError      bool
	}{
		{
			name: "valid request with chain denoms",
			queryParams: map[string]string{
				"base": "uosmo,uatom",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
						return mockPriceResult, nil
					},
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return true
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"uosmo":{"usdc":"1.250000000000000000000000000000000000"},"uatom":{"usdc":"10.500000000000000000000000000000000000"}}`,
		},
		{
			name: "valid request with human denoms",
			queryParams: map[string]string{
				"base":        "osmo,atom",
				"humanDenoms": "true",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
						return mockPriceResult, nil
					},
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return true
					},
					GetChainDenomFunc: func(humanDenom string) (string, error) {
						if humanDenom == "osmo" {
							return "uosmo", nil
						}
						if humanDenom == "atom" {
							return "uatom", nil
						}
						return "", nil
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"uosmo":{"usdc":"1.250000000000000000000000000000000000"},"uatom":{"usdc":"10.500000000000000000000000000000000000"}}`,
		},
		{
			name: "valid request with coingecko pricing source",
			queryParams: map[string]string{
				"base":          "uosmo",
				"pricingSource": "1",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
						return mockPriceResult, nil
					},
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return pricingSource == 1
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"uosmo":{"usdc":"1.250000000000000000000000000000000000"},"uatom":{"usdc":"10.500000000000000000000000000000000000"}}`,
		},
		{
			name: "valid request with debug mode",
			queryParams: map[string]string{
				"base":  "uosmo",
				"debug": "true",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
						return mockPriceResultWithRoutes, nil
					},
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return true
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"uosmo":{"usdc":{"price":"1.250000000000000000000000000000000000","routes":[{"pool_id":[1],"out_amount":"1000","in_amount":"800"}]}}}`,
		},
		{
			name:        "missing base parameter",
			queryParams: map[string]string{
				// no base parameter
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"denoms input must be non-empty"}`,
			expectedError:      true,
		},
		{
			name: "invalid base parameter format",
			queryParams: map[string]string{
				"base": "invalid-denom!",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"invalid denom: invalid-denom!"}`,
			expectedError:      true,
		},
		{
			name: "invalid pricing source",
			queryParams: map[string]string{
				"base":          "uosmo",
				"pricingSource": "999",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return false
					},
				},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"invalid pricing source: 999"}`,
			expectedError:      true,
		},
		{
			name: "invalid chain denom",
			queryParams: map[string]string{
				"base": "invalidchain",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return false
					},
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return true
					},
				},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"base denom is not a valid chain denom (invalidchain)"}`,
			expectedError:      true,
		},
		{
			name: "invalid chain denom in a batch is left out",
			queryParams: map[string]string{
				"base": "uosmo,invalidchain,uatom",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
						if len(baseDenoms) != 2 || baseDenoms[0] != "uosmo" || baseDenoms[1] != "uatom" {
							return nil, errors.New("unexpected base denoms " + strings.Join(baseDenoms, ","))
						}
						return mockPriceResult, nil
					},
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return chainDenom != "invalidchain"
					},
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return true
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"uosmo":{"usdc":"1.250000000000000000000000000000000000"},"uatom":{"usdc":"10.500000000000000000000000000000000000"}}`,
		},
		{
			name: "duplicate base denoms are deduplicated before pricing",
			queryParams: map[string]string{
				"base": "uosmo,uatom,uosmo,uatom,uosmo",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
						if len(baseDenoms) != 2 || baseDenoms[0] != "uosmo" || baseDenoms[1] != "uatom" {
							return nil, errors.New("unexpected base denoms " + strings.Join(baseDenoms, ","))
						}
						return mockPriceResult, nil
					},
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return true
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"uosmo":{"usdc":"1.250000000000000000000000000000000000"},"uatom":{"usdc":"10.500000000000000000000000000000000000"}}`,
		},
		{
			name: "invalid debug parameter",
			queryParams: map[string]string{
				"base":  "uosmo",
				"debug": "invalid",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"strconv.ParseBool: parsing \"invalid\": invalid syntax"}`,
			expectedError:      true,
		},
		{
			name: "invalid humanDenoms parameter",
			queryParams: map[string]string{
				"base":        "uosmo",
				"humanDenoms": "invalid",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"strconv.ParseBool: parsing \"invalid\": invalid syntax"}`,
			expectedError:      true,
		},
		{
			name: "GetPrices usecase error",
			queryParams: map[string]string{
				"base": "uosmo",
			},
			handler: &tokensdelivery.TokensHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
						return nil, errors.New("pricing service unavailable")
					},
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
					IsValidPricingSourceFunc: func(pricingSource int) bool {
						return true
					},
				},
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"message":"pricing service unavailable"}`,
			expectedError:      true,
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			e := deliveryhttp.NewEcho()
			req := httptest.NewRequest(echo.GET, "/", nil)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			q := req.URL.Query()
			for k, v := range tc.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := tc.handler.GetPrices(c)

			if tc.expectedError {
				// Note: in case of error, we expect err to be nil but the status code to be non-200
				s.Assert().Nil(err)
				s.Assert().Equal(tc.expectedStatusCode, rec.Code)
				s.Assert().JSONEq(tc.expectedResponse, rec.Body.String())
				return
			}

			s.Assert().NoError(err)
			s.Assert().Equal(tc.expectedStatusCode, rec.Code)
			s.Assert().JSONEq(
				strings.TrimSpace(tc.expectedResponse),
				strings.TrimSpace(rec.Body.String()),
			)
		})
	}
}

// denomList returns n distinct valid chain denoms joined by commas.
func denomList(n int) string {
	denoms := make([]string, 0, n)
	for i := range n {
		denoms = append(denoms, fmt.Sprintf("denom%d", i))
	}
	return strings.Join(denoms, ",")
}

// TestDenomsParamLimit verifies that every /tokens endpoint taking a denom list accepts exactly the
// maximum number of distinct denoms, passing each one to the usecase, and rejects one more before
// the usecase is reached, so an off-by-one in any bound check fails here.
func (s *TokensHandlerSuite) TestDenomsParamLimit() {
	const maxDenoms = 500

	// received counts the denoms that reached the usecase in the current subtest.
	var received int
	handler := &tokensdelivery.TokensHandler{
		TUsecase: &mocks.TokensUsecaseMock{
			IsValidChainDenomFunc: func(string) bool { return true },
			GetMetadataByChainDenomFunc: func(string) (domain.Token, error) {
				received++
				return domain.Token{}, nil
			},
			GetPoolDenomsMetadataFunc: func(chainDenoms []string) domain.PoolDenomMetaDataMap {
				received += len(chainDenoms)
				return domain.PoolDenomMetaDataMap{}
			},
			GetPricesFunc: func(_ context.Context, baseDenoms []string, _ []string, _ domain.PricingSourceType, _ ...domain.PricingOption) (domain.PricesResult, error) {
				received += len(baseDenoms)
				return domain.PricesResult{}, nil
			},
		},
	}

	endpoints := []struct {
		name  string
		param string
		call  func(echo.Context) error
	}{
		{name: "GetMetadata", param: "denoms", call: handler.GetMetadata},
		{name: "GetPoolDenomMetadata", param: "denoms", call: handler.GetPoolDenomMetadata},
		{name: "GetPrices", param: "base", call: handler.GetPrices},
	}

	cases := []struct {
		name        string
		count       int
		expCode     int
		expReceived int
	}{
		{name: "at limit is accepted", count: maxDenoms, expCode: http.StatusOK, expReceived: maxDenoms},
		{name: "over limit is rejected", count: maxDenoms + 1, expCode: http.StatusBadRequest, expReceived: 0},
	}

	for _, ep := range endpoints {
		for _, tc := range cases {
			s.Run(ep.name+" "+tc.name, func() {
				received = 0
				req := httptest.NewRequest(echo.GET, "/?"+ep.param+"="+denomList(tc.count), nil)
				rec := httptest.NewRecorder()

				s.Require().NoError(ep.call(deliveryhttp.NewEcho().NewContext(req, rec)))
				s.Require().Equal(tc.expCode, rec.Code, rec.Body.String())
				s.Require().Equal(tc.expReceived, received)
				if tc.expCode == http.StatusBadRequest {
					s.Require().JSONEq(`{"message":"too many denoms, max: 500"}`, rec.Body.String())
				}
			})
		}
	}
}
