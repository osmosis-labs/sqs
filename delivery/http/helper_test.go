package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestParseBooleanQueryParam(t *testing.T) {
	tests := []struct {
		name          string
		queryParam    string
		expectedValue bool
		expectedError bool
	}{
		{"True value", "param=true", true, false},
		{"False value", "param=false", false, false},
		{"1 as true", "param=1", true, false},
		{"0 as false", "param=0", false, false},
		{"Empty value", "", false, false},
		{"Invalid value", "param=invalid", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/?"+tt.queryParam, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Test
			value, err := ParseBooleanQueryParam(c, "param")

			// Assert
			if tt.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedValue, value)
			}
		})
	}
}
