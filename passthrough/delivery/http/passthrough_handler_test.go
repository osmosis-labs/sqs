package http_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/orderbook/types"
	"github.com/osmosis-labs/sqs/orderbook/usecase/orderbooktesting"
	passthroughdelivery "github.com/osmosis-labs/sqs/passthrough/delivery/http"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PassthroughHandlerTestSuite struct {
	orderbooktesting.OrderbookTestHelper
}

func TestPassthroughHandlerSuite(t *testing.T) {
	suite.Run(t, new(PassthroughHandlerTestSuite))
}

func (s *PassthroughHandlerTestSuite) TestGetActiveOrders() {
	testCases := []struct {
		name               string
		queryParams        map[string]string
		setupMocks         func(usecase *mocks.OrderbookUsecaseMock)
		expectedStatusCode int
		expectedResponse   string
		expectedError      bool
	}{
		{
			name:               "validation error",
			queryParams:        map[string]string{},
			setupMocks:         func(usecase *mocks.OrderbookUsecaseMock) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   fmt.Sprintf(`{"message":"%s"}`, types.ErrUserOsmoAddressInvalid.Error()),
			expectedError:      true,
		},
		{
			name: "returns a few active orders",
			queryParams: map[string]string{
				"userOsmoAddress": "osmo1ugku28hwyexpljrrmtet05nd6kjlrvr9jz6z00",
			},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetActiveOrdersFunc = func(ctx context.Context, address string) ([]orderbookdomain.LimitOrder, bool, error) {
					return []orderbookdomain.LimitOrder{
						s.NewLimitOrder().WithOrderID(1).LimitOrder,
						s.NewLimitOrder().WithOrderID(2).LimitOrder,
					}, false, nil
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   s.MustReadFile("../../../orderbook/usecase/orderbooktesting/parsing/active_orders_response.json"),
			expectedError:      false,
		},
		{
			name: "internal server error from usecase",
			queryParams: map[string]string{
				"userOsmoAddress": "osmo1ev0vtddkl7jlwfawlk06yzncapw2x9quva4wzw",
			},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetActiveOrdersFunc = func(ctx context.Context, address string) ([]orderbookdomain.LimitOrder, bool, error) {
					return nil, false, assert.AnError
				}
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   fmt.Sprintf(`{"message":"%s"}`, types.ErrInternalError.Error()),
			expectedError:      true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			e := echo.New()
			req := httptest.NewRequest(echo.GET, "/", nil)
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			q := req.URL.Query()
			for k, v := range tc.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Set up the mocks
			usecase := mocks.OrderbookUsecaseMock{}
			if tc.setupMocks != nil {
				tc.setupMocks(&usecase)
			}

			// Initialize the handler with mocked usecase
			handler := passthroughdelivery.PassthroughHandler{OUsecase: &usecase}

			// Call the method under test
			err := handler.GetActiveOrders(c)

			// Check the error condition
			if tc.expectedError {
				s.Assert().Nil(err)
			} else {
				s.Assert().NoError(err)
			}

			// Check the response
			s.Assert().Equal(tc.expectedStatusCode, rec.Code)
			s.Assert().JSONEq(tc.expectedResponse, strings.TrimSpace(rec.Body.String()))
		})
	}
}
