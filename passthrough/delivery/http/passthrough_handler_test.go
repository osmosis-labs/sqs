package http_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/log"
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

func (s *PassthroughHandlerTestSuite) TestGetActiveOrdersStream() {
	eventData := func(data string) string {
		data = strings.ReplaceAll(strings.ReplaceAll(data, "\n", ""), " ", "")

		event := deliveryhttp.Event{
			Data: []byte(data),
		}

		w := bytes.NewBuffer(nil)
		err := event.MarshalTo(w)
		s.Assert().NoError(err)

		return w.String()
	}

	testCases := []struct {
		name               string
		queryParams        map[string]string
		setupMocks         func(usecase *mocks.OrderbookUsecaseMock)
		expectedStatusCode int
		expectedResponse   string
	}{
		{
			name:               "validation error: missing userOsmoAddress",
			queryParams:        map[string]string{}, // missing userOsmoAddress
			setupMocks:         func(usecase *mocks.OrderbookUsecaseMock) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   fmt.Sprintf(`{"message":"%s"}`+"\n", types.ErrUserOsmoAddressInvalid.Error()),
		},
		{
			name: "validation error: invalid userOsmoAddress",
			queryParams: map[string]string{
				"userOsmoAddress": "notvalid",
			},
			setupMocks:         func(usecase *mocks.OrderbookUsecaseMock) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   fmt.Sprintf(`{"message":"%s"}`+"\n", types.ErrUserOsmoAddressInvalid.Error()),
		},
		{
			name: "returns active orders stream",
			queryParams: map[string]string{
				"userOsmoAddress": "osmo1ugku28hwyexpljrrmtet05nd6kjlrvr9jz6z00",
			},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetActiveOrdersStreamFunc = func(ctx context.Context, address string) <-chan orderbookdomain.OrderbookResult {
					ordersCh := make(chan orderbookdomain.OrderbookResult)
					go func(c chan orderbookdomain.OrderbookResult) {
						c <- orderbookdomain.OrderbookResult{
							LimitOrders: []orderbookdomain.LimitOrder{
								s.NewLimitOrder().WithOrderID(1).LimitOrder,
								s.NewLimitOrder().WithOrderID(2).LimitOrder,
							},
							IsBestEffort: false,
							Error:        nil,
						}
						close(c)
					}(ordersCh)
					return ordersCh
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   eventData(s.MustReadFile("../../../orderbook/usecase/orderbooktesting/parsing/active_orders_response.json")),
		},
		{
			name: "internal server error during stream",
			queryParams: map[string]string{
				"userOsmoAddress": "osmo1ev0vtddkl7jlwfawlk06yzncapw2x9quva4wzw",
			},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				ordersCh := make(chan orderbookdomain.OrderbookResult)
				usecase.GetActiveOrdersStreamFunc = func(ctx context.Context, address string) <-chan orderbookdomain.OrderbookResult {
					go func() {
						ordersCh <- orderbookdomain.OrderbookResult{
							LimitOrders:  nil,
							IsBestEffort: false,
							Error:        assert.AnError,
						}
						close(ordersCh)
					}()
					return ordersCh
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   "id: \ndata: {\"orders\":[],\"is_best_effort\":false}\n\n",
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

			c.Request().Context().Done()
			// Set up the mocks
			usecase := mocks.OrderbookUsecaseMock{}
			if tc.setupMocks != nil {
				tc.setupMocks(&usecase)
			}

			// Initialize the handler with mocked usecase
			handler := passthroughdelivery.PassthroughHandler{
				OUsecase: &usecase,
				Logger:   &log.NoOpLogger{},
			}

			// Call the method under test
			err := handler.GetActiveOrdersStream(c)
			s.Assert().NoError(err)

			// Check the response
			s.Assert().Equal(tc.expectedStatusCode, rec.Code)
			s.Assert().Equal(tc.expectedResponse, rec.Body.String())
		})
	}
}
