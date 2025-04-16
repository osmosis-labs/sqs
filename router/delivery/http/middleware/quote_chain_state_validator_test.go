package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/stretchr/testify/assert"
)

func TestQuoteChainStateValidatorMiddleware(t *testing.T) {
	tests := []struct {
		name                 string
		getLatestHeightErr   error
		validateUpdatesErr   error
		expectedStatusCode   int
		expectedResponseBody string
	}{
		{
			name:               "Success",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:                 "GetLatestHeight Error",
			getLatestHeightErr:   errors.New("failed to get latest height"),
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: `{"message":"no candidate routes found"}`,
		},
		{
			name:                 "ValidateCandidateRouteSearchDataUpdates Error",
			validateUpdatesErr:   errors.New("failed to validate updates"),
			expectedStatusCode:   http.StatusInternalServerError,
			expectedResponseBody: `{"message":"no candidate routes found"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			mockChainUsecase := &mocks.ChainInfoUsecaseMock{
				GetLatestHeightFunc: func() (uint64, error) {
					return 0, tt.getLatestHeightErr
				},
				ValidateCandidateRouteSearchDataUpdatesFunc: func() error {
					return tt.validateUpdatesErr
				},
			}

			middleware := QuoteChainStateValidatorMiddleware(mockChainUsecase)

			// Test
			handler := middleware(func(c echo.Context) error {
				return c.String(http.StatusOK, "OK")
			})

			err := handler(c)

			// Assert
			if tt.expectedStatusCode == http.StatusOK {
				assert.NoError(t, err)
				assert.Equal(t, http.StatusOK, rec.Code)
				assert.Equal(t, "OK", rec.Body.String())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatusCode, rec.Code)
				assert.JSONEq(t, tt.expectedResponseBody, rec.Body.String())
			}
		})
	}
}
