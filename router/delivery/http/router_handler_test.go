package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
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

func (s *RouterHandlerSuite) TestGetOptimalQuote() {
	// Prepare 3 pools, we create once and reuse them in the test cases
	// It's done to avoid creating them multiple times and increasing pool IDs counter.
	_, poolOne := s.PoolOne()
	_, poolTwo := s.PoolTwo()
	_, poolThree := s.PoolThree()

	testcases := []struct {
		name               string
		queryParams        map[string]string
		handler            *routerdelivery.RouterHandler
		expectedStatusCode int
		expectedResponse   string
		expectedError      bool
	}{
		{
			name: "valid exact in request",
			queryParams: map[string]string{
				"tokenIn":        "1000ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5",
				"tokenOutDenom":  "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			handler: &routerdelivery.RouterHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
				},
				RUsecase: &mocks.RouterUsecaseMock{
					GetOptimalQuoteFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
						return s.NewExactAmountInQuote(poolOne, poolTwo, poolThree), nil
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_in_response.json"),
		},
		{
			name: "valid exact in request (simulated)",
			queryParams: map[string]string{
				"tokenIn":                     "1000ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5",
				"tokenOutDenom":               "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
				"singleRoute":                 "true",
				"applyExponents":              "true",
				"simulatorAddress":            "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
				"simulationSlippageTolerance": "1.01",
			},
			handler: &routerdelivery.RouterHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
				},
				RUsecase: &mocks.RouterUsecaseMock{
					GetOptimalQuoteFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
						return s.NewExactAmountInQuote(poolOne, poolTwo, poolThree), nil
					},
				},
				QuoteSimulator: &mocks.QuoteSimulatorMock{
					SimulateQuoteFn: func(ctx context.Context, quote domain.Quote, slippageToleranceMultiplier math.LegacyDec, simulatorAddress string) domain.TxFeeInfo {
						return domain.TxFeeInfo{
							AdjustedGasUsed: 1_000_000,
							FeeCoin:         sdk.NewCoin("uosmo", math.NewInt(1000)),
							BaseFee:         osmomath.NewDecWithPrec(5, 1),
						}
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_in_response_simulated.json"),
		},
		{
			name: "valid exact in request with base fee",
			queryParams: map[string]string{
				"tokenIn":        "1000ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5",
				"tokenOutDenom":  "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
				"singleRoute":    "true",
				"applyExponents": "true",
				"appendBaseFee":  "true",
			},
			handler: &routerdelivery.RouterHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
				},
				RUsecase: &mocks.RouterUsecaseMock{
					GetOptimalQuoteFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
						return s.NewExactAmountInQuote(poolOne, poolTwo, poolThree), nil
					},

					BaseFee: domain.BaseFee{
						Denom:      "uosmo",
						CurrentFee: osmomath.NewDecWithPrec(5, 1),
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_in_response_base_fee.json"),
		},
		{
			name: "valid exact out request",
			queryParams: map[string]string{
				"tokenOut":       "1000ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
				"tokenInDenom":   "ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			handler: &routerdelivery.RouterHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
				},
				RUsecase: &mocks.RouterUsecaseMock{
					GetOptimalQuoteInGivenOutFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
						return s.NewExactAmountOutQuote(poolOne, poolTwo, poolThree), nil
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_out_response.json"),
		},
		{
			name: "invalid swap method request",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOut":       "1000usdc",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message": "swap method is invalid - must be either swap exact amount in or swap exact amount out"}`,
			expectedError:      true,
		},
		{
			name: "invalid tokenIn format",
			queryParams: map[string]string{
				"tokenIn":        "invalid_denom",
				"tokenOutDenom":  "usdc",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message": "tokenIn is invalid - must be in the format amountDenom"}`,
			expectedError:      true,
		},
		{
			name: "invalid tokenOut format",
			queryParams: map[string]string{
				"tokenInDenom":   "ust",
				"tokenOut":       "invalid_denom",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message": "tokenOut is invalid - must be in the format amountDenom"}`,
			expectedError:      true,
		},
	}
	for _, tc := range testcases {
		s.Run(tc.name, func() {
			e := echo.New()
			req := httptest.NewRequest(echo.POST, "/", nil)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			q := req.URL.Query()
			for k, v := range tc.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := tc.handler.GetOptimalQuote(c)

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

func (s *RouterHandlerSuite) TestGetDirectCustomQuote() {
	// Prepare 3 pools, we create once and reuse them in the test cases
	// It's done to avoid creating them multiple times and increasing pool IDs counter.
	_, poolOne := s.PoolOne()
	_, poolTwo := s.PoolTwo()
	_, poolThree := s.PoolThree()

	testcases := []struct {
		name               string
		queryParams        map[string]string
		handler            *routerdelivery.RouterHandler
		expectedStatusCode int
		expectedResponse   string
		expectedError      bool
	}{
		{
			name: "valid exact in request",
			queryParams: map[string]string{
				"tokenIn":        "1000ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5",
				"tokenOutDenom":  "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
				"poolID":         "10",
				"applyExponents": "true",
			},
			handler: &routerdelivery.RouterHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
				},
				RUsecase: &mocks.RouterUsecaseMock{
					GetCustomDirectQuoteMultiPoolFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom []string, poolIDs []uint64) (domain.Quote, error) {
						return s.NewExactAmountInQuote(poolOne, poolTwo, poolThree), nil
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_in_response.json"),
		},
		{
			name: "valid exact out request",
			queryParams: map[string]string{
				"tokenOut":       "1000ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4",
				"tokenInDenom":   "ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5",
				"poolID":         "10",
				"applyExponents": "true",
			},
			handler: &routerdelivery.RouterHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						return true
					},
				},
				RUsecase: &mocks.RouterUsecaseMock{
					GetCustomDirectQuoteMultiPoolInGivenOutFunc: func(ctx context.Context, tokenOut sdk.Coin, tokenInDenom []string, poolIDs []uint64) (domain.Quote, error) {
						return s.NewExactAmountOutQuote(poolOne, poolTwo, poolThree), nil
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_out_response.json"),
		},
		{
			name: "valid exact out request: apply human denom",
			queryParams: map[string]string{
				"tokenOut":       "1000usdc",
				"tokenInDenom":   "eth",
				"poolID":         "10",
				"applyExponents": "true",
				"humanDenoms":    "true",
			},
			handler: &routerdelivery.RouterHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						// because we are applying human denoms
						// test will fail with humanDenoms set to false
						return false
					},
				},
				RUsecase: &mocks.RouterUsecaseMock{
					GetCustomDirectQuoteMultiPoolInGivenOutFunc: func(ctx context.Context, tokenOut sdk.Coin, tokenInDenom []string, poolIDs []uint64) (domain.Quote, error) {
						return s.NewExactAmountOutQuote(poolOne, poolTwo, poolThree), nil
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_out_response.json"),
		},
		{
			name: "not valid exact out request: apply human denom",
			queryParams: map[string]string{
				"tokenOut":       "1000usdc",
				"tokenInDenom":   "eth",
				"poolID":         "10",
				"applyExponents": "true",
				"humanDenoms":    "false",
			},
			handler: &routerdelivery.RouterHandler{
				TUsecase: &mocks.TokensUsecaseMock{
					IsValidChainDenomFunc: func(chainDenom string) bool {
						// because we are applying human denoms
						// test will fail with humanDenoms set to false
						return false
					},
				},
				RUsecase: &mocks.RouterUsecaseMock{
					GetCustomDirectQuoteMultiPoolInGivenOutFunc: func(ctx context.Context, tokenOut sdk.Coin, tokenInDenom []string, poolIDs []uint64) (domain.Quote, error) {
						return s.NewExactAmountOutQuote(poolOne, poolTwo, poolThree), nil
					},
				},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"denom is not a valid chain denom (usdc)"}`,
			expectedError:      true,
		},
		{
			name: "invalid swap method request",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOutDenom":  "usdc",
				"tokenOut":       "1000usdc",
				"tokenInDenom":   "atom",
				"poolID":         "10",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"swap method is invalid - must be either swap exact amount in or swap exact amount out"}`,
			expectedError:      true,
		},
		{
			name: "invalid pools request",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOutDenom":  "usdc",
				"tokenOut":       "1000usdc",
				"tokenInDenom":   "atom",
				"poolID":         "string,5",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"pool ID must be integer"}`,
			expectedError:      true,
		},
		{
			name: "invalid tokenIn format",
			queryParams: map[string]string{
				"tokenIn":        "invalid_denom",
				"tokenOutDenom":  "usdc",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"tokenIn is invalid - must be in the format amountDenom"}`,
			expectedError:      true,
		},
		{
			name: "invalid tokenOut format",
			queryParams: map[string]string{
				"tokenInDenom":   "ust",
				"tokenOut":       "invalid_denom",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"tokenOut is invalid - must be in the format amountDenom"}`,
			expectedError:      true,
		},
	}

	for _, tc := range testcases {
		s.Run(tc.name, func() {
			e := echo.New()
			req := httptest.NewRequest(echo.POST, "/", nil)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			q := req.URL.Query()
			for k, v := range tc.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := tc.handler.GetDirectCustomQuote(c)

			if tc.expectedError {
				// Note: in case of error, we expect err to be nil but the status code to be non-200
				s.Assert().Nil(err)
				s.Assert().Equal(tc.expectedStatusCode, rec.Code)
				s.Assert().Equal(
					strings.TrimSpace(tc.expectedResponse),
					strings.TrimSpace(rec.Body.String()),
				)
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
