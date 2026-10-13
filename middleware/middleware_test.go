package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/middleware"
)

// TestRecoverMiddleware checks that a panicking handler returns a generic 500
// without the panic text, and that the server keeps serving afterwards.
func TestRecoverMiddleware(t *testing.T) {
	e := echo.New()
	m := middleware.InitMiddleware(&domain.CORSConfig{}, &domain.FlightRecordConfig{}, &log.NoOpLogger{})
	e.Use(m.RecoverMiddleware())
	e.GET("/panic", func(c echo.Context) error {
		panic("secret internal detail")
	})
	e.GET("/ok", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	server := httptest.NewServer(e)
	defer server.Close()

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"message":"Internal Server Error"}`, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "secret internal detail")

	// Over a real connection: the panic must not take the server down.
	resp, err := http.Get(server.URL + "/panic")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	resp, err = http.Get(server.URL + "/ok")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}
