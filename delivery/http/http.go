package http

import (
	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/validator"
)

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
