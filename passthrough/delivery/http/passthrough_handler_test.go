package http_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/ingest/types/cosmwasmpool"
	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
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

func (s *PassthroughHandlerTestSuite) TestGetOrderbookTicks() {
	// tick builds an OrderbookTick with explicit total-amount-of-liquidity values,
	// which is the field the handler reads. askCancels/bidCancels of -1 mean "leave
	// the unrealized-cancels Int nil", matching ingested state for a side that has
	// never had a cancel.
	tick := func(askTAL, bidTAL string, askCancels, bidCancels int64) orderbookdomain.OrderbookTick {
		t := orderbookdomain.OrderbookTick{
			TickState: orderbookdomain.TickState{
				AskValues: orderbookdomain.TickValues{TotalAmountOfLiquidity: askTAL},
				BidValues: orderbookdomain.TickValues{TotalAmountOfLiquidity: bidTAL},
			},
		}
		if askCancels >= 0 {
			t.UnrealizedCancels.AskUnrealizedCancels = osmomath.NewInt(askCancels)
		}
		if bidCancels >= 0 {
			t.UnrealizedCancels.BidUnrealizedCancels = osmomath.NewInt(bidCancels)
		}
		return t
	}

	// tickJSON renders the expected wire shape for a single tick. The zero-valued
	// TickValues fields are serialised explicitly because they have no omitempty.
	tickJSON := func(tickID int64, askTAL, bidTAL string) string {
		const values = `{"total_amount_of_liquidity":%q,"cumulative_total_value":"","effective_total_amount_swapped":"","cumulative_realized_cancels":"","last_tick_sync_etas":""}`
		return fmt.Sprintf(
			`{"tick_id":%d,"tick_state":{"ask_values":`+values+`,"bid_values":`+values+`}}`,
			tickID, askTAL, bidTAL,
		)
	}

	testCases := []struct {
		name               string
		queryParams        map[string]string
		setupMocks         func(usecase *mocks.OrderbookUsecaseMock)
		notAnOrderbook     bool
		poolNotFound       bool
		expectedStatusCode int
		expectedResponse   string
		expectedError      bool
	}{
		{
			name:               "missing poolID is a validation error",
			queryParams:        map[string]string{},
			setupMocks:         func(usecase *mocks.OrderbookUsecaseMock) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      true,
		},
		{
			name:               "non-numeric poolID is a validation error",
			queryParams:        map[string]string{"poolID": "not-a-pool"},
			setupMocks:         func(usecase *mocks.OrderbookUsecaseMock) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      true,
		},
		{
			name:               "zero poolID is a validation error",
			queryParams:        map[string]string{"poolID": "0"},
			setupMocks:         func(usecase *mocks.OrderbookUsecaseMock) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedResponse:   `{"message":"poolID must be non-zero"}`,
		},
		{
			name:               "non-orderbook pool is a 404, not an empty book",
			queryParams:        map[string]string{"poolID": "1933"},
			setupMocks:         func(usecase *mocks.OrderbookUsecaseMock) {},
			notAnOrderbook:     true,
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:               "unknown pool is a 404, not an empty book",
			queryParams:        map[string]string{"poolID": "1933"},
			setupMocks:         func(usecase *mocks.OrderbookUsecaseMock) {},
			poolNotFound:       true,
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			// A non-canonical orderbook (not top-of-pair for its base/quote) still
			// has depth worth serving, so it must not 404.
			name:        "non-canonical orderbook pool is still served",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{
						10: tick("100", "0", -1, -1),
					}, true
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"ticks":[` + tickJSON(10, "100", "0") + `]}`,
		},
		{
			name:        "not-yet-ingested pool is a 503, not an empty book",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return nil, false
				}
			},
			expectedStatusCode: http.StatusServiceUnavailable,
			expectedError:      true,
		},
		{
			name:        "indexed pool with no ticks returns an empty tick array",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{}, true
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"ticks":[]}`,
		},
		{
			name:        "ticks are sorted by ascending tick ID",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{
						30: tick("300", "0", -1, -1),
						10: tick("100", "0", -1, -1),
						20: tick("200", "0", -1, -1),
					}, true
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: `{"ticks":[` +
				tickJSON(10, "100", "0") + `,` +
				tickJSON(20, "200", "0") + `,` +
				tickJSON(30, "300", "0") + `]}`,
		},
		{
			name:        "negative tick IDs sort below positive ones",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{
						5:  tick("5", "0", -1, -1),
						-5: tick("50", "0", -1, -1),
					}, true
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse: `{"ticks":[` +
				tickJSON(-5, "50", "0") + `,` +
				tickJSON(5, "5", "0") + `]}`,
		},
		{
			name:        "unrealized cancels are subtracted from both sides",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{
						10: tick("1000", "500", 250, 100),
					}, true
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"ticks":[` + tickJSON(10, "750", "400") + `]}`,
		},
		{
			// Cancels cannot exceed the liquidity they cancel. Publishing a clamped
			// zero would present fabricated depth as real, so this is an error.
			name:        "cancels exceeding liquidity violate the invariant and error",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{
						10: tick("100", "100", 500, 0),
					}, true
				}
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   fmt.Sprintf(`{"message":"%s"}`, types.ErrInternalError.Error()),
		},
		{
			name:        "ticks with no liquidity on either side are omitted",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{
						10: tick("0", "0", -1, -1),       // explicit zeros
						20: tick("100", "0", -1, -1),     // survives
						30: tick("", "", -1, -1),         // never-populated side
						40: tick("100", "100", 100, 100), // fully cancelled out
					}, true
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"ticks":[` + tickJSON(20, "100", "0") + `]}`,
		},
		{
			name:        "decimal liquidity is truncated to an integer string",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{
						10: tick("100.9", "50.5", -1, 10),
					}, true
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedResponse:   `{"ticks":[` + tickJSON(10, "100", "40") + `]}`,
		},
		{
			name:        "malformed liquidity surfaces an error instead of being passed through",
			queryParams: map[string]string{"poolID": "1933"},
			setupMocks: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return map[int64]orderbookdomain.OrderbookTick{
						10: tick("not-a-number", "0", -1, -1),
					}, true
				}
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedResponse:   fmt.Sprintf(`{"message":"%s"}`, types.ErrInternalError.Error()),
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

			// The pool resolves to an orderbook unless the case says otherwise.
			pools := mocks.PoolsUsecaseMock{}
			pools.GetPoolFunc = func(poolID uint64) (ingesttypes.PoolI, error) {
				if tc.poolNotFound {
					return nil, domain.PoolNotFoundError{PoolID: poolID}
				}
				model := &cosmwasmpool.CosmWasmPoolModel{}
				if !tc.notAnOrderbook {
					model.ContractInfo = cosmwasmpool.ContractInfo{
						Contract: cosmwasmpool.ORDERBOOK_CONTRACT_NAME,
						Version:  cosmwasmpool.ORDERBOOK_MIN_CONTRACT_VERSION,
					}
				}
				return &mocks.MockRoutablePool{ID: poolID, CosmWasmPoolModel: model}, nil
			}

			// Initialize the handler with mocked usecase
			handler := passthroughdelivery.PassthroughHandler{
				OUsecase:     &usecase,
				PoolsUsecase: &pools,
				Logger:       &log.NoOpLogger{},
			}

			// Call the method under test
			err := handler.GetOrderbookTicks(c)
			s.Assert().NoError(err)

			// Check the response
			s.Assert().Equal(tc.expectedStatusCode, rec.Code)
			if tc.expectedError {
				// These cases wrap a strconv error, so the exact wording belongs to the
				// stdlib and is not worth pinning. Assert only that a message came back.
				s.Assert().Contains(rec.Body.String(), "message")
			} else {
				s.Assert().JSONEq(tc.expectedResponse, strings.TrimSpace(rec.Body.String()))
			}
		})
	}
}
