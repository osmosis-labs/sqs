package http

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/validator"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// DefaultClient represents default HTTP client for issuing outgoing HTTP requests.
var DefaultClient = &http.Client{
	Timeout:   5 * time.Second, // Adjusted timeout to 5 seconds
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

// RequestUnmarshaler is any type capable to unmarshal data from HTTP request to itself.
type RequestUnmarshaler interface {
	UnmarshalHTTPRequest(c echo.Context) error
}

// UnmarshalRequest unmarshals HTTP request into m.
func UnmarshalRequest(c echo.Context, m RequestUnmarshaler) error {
	return m.UnmarshalHTTPRequest(c)
}

// ParseRequest encapsulates the request unmarshalling and validation logic.
// It unmarshals the request and validates it if the request implements the Validator interface.
func ParseRequest(c echo.Context, req RequestUnmarshaler) error {
	if err := UnmarshalRequest(c, req); err != nil {
		return err
	}

	v, ok := req.(validator.Validator)
	if !ok {
		return nil
	}
	return validator.Validate(v)
}
