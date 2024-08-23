package http

import (
	"github.com/labstack/echo/v4"
)

// RequestUnmarshaler is any type capable to unmarshal data from HTTP request to itself.
type RequestUnmarshaler interface {
	UnmarshalHTTPRequest(c echo.Context) error
}

// UnmarshalRequest unmarshals HTTP request into m.
func UnmarshalRequest(c echo.Context, m RequestUnmarshaler) error {
	return m.UnmarshalHTTPRequest(c)
}
