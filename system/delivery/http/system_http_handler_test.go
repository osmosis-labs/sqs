package http_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	chaininfoclient "github.com/osmosis-labs/sqs/chaininfo/client"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	deliveryhttp "github.com/osmosis-labs/sqs/system/delivery/http"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHealthStatus(t *testing.T) {
	tests := []struct {
		name               string
		setupMock          func(*mocks.ChainInfoUsecaseMock)
		expectedStatusCode int
		expectedBody       string
	}{
		{
			name: "Healthy status",
			setupMock: func(mock *mocks.ChainInfoUsecaseMock) {
				mock.GetLatestHeightFunc = func() (uint64, error) {
					return 100, nil
				}
				mock.ValidatePriceUpdatesFunc = func() error {
					return nil
				}
				mock.ValidatePoolLiquidityUpdatesFunc = func() error {
					return nil
				}
				mock.ValidateCandidateRouteSearchDataUpdatesFunc = func() error {
					return nil
				}
				mock.GetClientFunc = func() chaininfoclient.Client {
					return &mocks.ChainInfoClientMock{
						GetStatusFunc: func(ctx context.Context) (*ctypes.ResultStatus, error) {
							return &ctypes.ResultStatus{
								SyncInfo: ctypes.SyncInfo{
									LatestBlockHeight: 105,
									CatchingUp:        false,
								},
							}, nil
						},
					}
				}
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       `{"chain_latest_height":"105","grpc_gateway_status":"running","store_latest_height":"100"}`,
		},
		{
			name: "Error connecting to the Osmosis chain via GRPC gateway",
			setupMock: func(mock *mocks.ChainInfoUsecaseMock) {
				mock.GetClientFunc = func() chaininfoclient.Client {
					return &mocks.ChainInfoClientMock{
						GetStatusFunc: func(ctx context.Context) (*ctypes.ResultStatus, error) {
							return nil, fmt.Errorf("client error")
						},
					}
				}
			},
			expectedStatusCode: http.StatusServiceUnavailable,
			expectedBody:       `Error connecting to the Osmosis chain via GRPC gateway`,
		},
		{
			name: "Failed to get latest height from sqs store",
			setupMock: func(mock *mocks.ChainInfoUsecaseMock) {
				mock.GetLatestHeightFunc = func() (uint64, error) {
					return 0, fmt.Errorf("error getting latest height from store")
				}
				mock.GetClientFunc = func() chaininfoclient.Client {
					return &mocks.ChainInfoClientMock{
						GetStatusFunc: func(ctx context.Context) (*ctypes.ResultStatus, error) {
							return &ctypes.ResultStatus{}, nil
						},
					}
				}
			},
			expectedStatusCode: http.StatusInternalServerError,
			expectedBody:       `Failed to get latest height from sqs store: error getting latest height from store`,
		},
		{
			name: "Node catching up",
			setupMock: func(mock *mocks.ChainInfoUsecaseMock) {
				mock.GetClientFunc = func() chaininfoclient.Client {
					return &mocks.ChainInfoClientMock{
						GetStatusFunc: func(ctx context.Context) (*ctypes.ResultStatus, error) {
							return &ctypes.ResultStatus{
								SyncInfo: ctypes.SyncInfo{
									CatchingUp: true,
								},
							}, nil
						},
					}
				}
			},
			expectedStatusCode: http.StatusServiceUnavailable,
			expectedBody:       `Node is still catching up`,
		},
		{
			name: "Node not synced",
			setupMock: func(mock *mocks.ChainInfoUsecaseMock) {
				mock.GetLatestHeightFunc = func() (uint64, error) {
					return 100, nil
				}
				mock.GetClientFunc = func() chaininfoclient.Client {
					return &mocks.ChainInfoClientMock{
						GetStatusFunc: func(ctx context.Context) (*ctypes.ResultStatus, error) {
							return &ctypes.ResultStatus{
								SyncInfo: ctypes.SyncInfo{
									LatestBlockHeight: 120,
									CatchingUp:        false,
								},
							}, nil
						},
					}
				}
			},
			expectedStatusCode: http.StatusServiceUnavailable,
			expectedBody:       `Node is not synced, chain height (120), store height (100), tolerance (10)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			mock := &mocks.ChainInfoUsecaseMock{}
			if tt.setupMock != nil {
				tt.setupMock(mock)
			}

			handler := &deliveryhttp.SystemHandler{
				Logger:    &log.NoOpLogger{},
				CIUsecase: mock,
			}

			// Act
			err := handler.GetHealthStatus(c)

			// Assert
			if tt.expectedStatusCode == http.StatusOK {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				httpError, ok := err.(*echo.HTTPError)
				assert.True(t, ok)
				assert.Equal(t, tt.expectedStatusCode, httpError.Code)
				assert.Equal(t, tt.expectedBody, string(httpError.Message.(string)))
			}

			if tt.expectedStatusCode == http.StatusOK {
				assert.Equal(t, tt.expectedStatusCode, rec.Code)
				assert.JSONEq(t, tt.expectedBody, rec.Body.String())
			}
		})
	}
}

func TestExtractVersion(t *testing.T) {
	// Test cases
	testCases := []struct {
		name            string
		ldFlagsValue    string
		expectedVersion string
	}{
		{
			name:         "version is specified first in the ldFlagsValue",
			ldFlagsValue: "-X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8     -w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static'",

			expectedVersion: "0.1.2-4-g79c82c8",
		},
		{
			name:         "version is specified in the end of ldFlagsValue",
			ldFlagsValue: "-w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static' -X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8",

			expectedVersion: "0.1.2-4-g79c82c8",
		},
		{
			name:         "version is specified in the middle of ldFlagsValue",
			ldFlagsValue: "-extldflags '-Wl,-z,muldefs -static' -X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8 -w -s -linkmode=external",

			expectedVersion: "0.1.2-4-g79c82c8",
		},
		{
			name:         "ldFlagsValue only version",
			ldFlagsValue: "-X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8",

			expectedVersion: "0.1.2-4-g79c82c8",
		},
	}

	// Run tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := deliveryhttp.ExtractVersion(tc.ldFlagsValue)
			require.NoError(t, err)

			require.Equal(t, tc.expectedVersion, result)
		})
	}
}
