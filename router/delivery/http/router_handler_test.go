package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	routerdelivery "github.com/osmosis-labs/sqs/router/delivery/http"
	routertypes "github.com/osmosis-labs/sqs/router/types"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"
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
	testcases := []struct {
		name               string
		queryParams        map[string]string
		handler            *routerdelivery.RouterHandler
		expectedStatusCode int
		expectedResponse   string
		expectedError      error
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
				TUsecase: tokensusecase.NewTokensUsecase(nil, 0, nil),
				RUsecase: &mocks.RouterUsecaseMock{
					GetOptimalQuoteFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
						_, p1 := s.PoolOne()
						_, p2 := s.PoolTwo()
						_, p3 := s.PoolThree()
						return s.NewExactAmountInQuote(p1, p2, p3), nil
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_in_response.json"),
			expectedError:      nil,
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
				TUsecase: tokensusecase.NewTokensUsecase(nil, 0, nil),
				RUsecase: &mocks.RouterUsecaseMock{
					GetOptimalQuoteInGivenOutFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
						_, p1 := s.PoolOne()
						_, p2 := s.PoolTwo()
						_, p3 := s.PoolThree()
						return s.NewExactAmountOutQuote(p1, p2, p3), nil
					},
				},
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../usecase/routertesting/parsing/quote_amount_out_response.json"),
			expectedError:      nil,
		},
		{
			name: "invalid swap method request",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOut":       "1000usdc",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"message": "swap method is invalid - must be either swap exact amount in or swap exact amount out"}`,
			expectedError:      routertypes.ErrSwapMethodNotValid,
		},
		{
			name: "invalid tokenIn format",
			queryParams: map[string]string{
				"tokenIn":        "invalid_denom",
				"tokenOutDenom":  "usdc",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"message": "tokenIn is invalid - must be in the format amountDenom"}`,
			expectedError:      routertypes.ErrTokenInNotValid,
		},
		{
			name: "invalid tokenOut format",
			queryParams: map[string]string{
				"tokenInDenom":   "ust",
				"tokenOut":       "invalid_denom",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   `{"message": "tokenOut is invalid - must be in the format amountDenom"}`,
			expectedError:      routertypes.ErrTokenOutNotValid,
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

			if tc.expectedError != nil {
				s.Assert().Error(err)
				s.Assert().Equal(tc.expectedError, err)
				s.Assert().Equal(tc.expectedStatusCode, rec.Code)
				s.Assert().JSONEq(tc.expectedResponse, rec.Body.String())
				return
			}

			s.Assert().NoError(err)
			s.Assert().Equal(tc.expectedStatusCode, rec.Code)
			s.Assert().Equal(tc.expectedResponse, strings.TrimSpace(rec.Body.String())) // JSONEq fails
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
			err:  routertypes.ErrTokenInNotSpecified,
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
			err:  routertypes.ErrNumOfTokenOutDenomPoolsMismatch,
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
