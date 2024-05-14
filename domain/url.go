package domain

import (
	"context"
	"net/url"
	"strconv"

	"github.com/labstack/echo/v4"
)

// RequestPathKeyType is a custom type for request path key.
type RequestPathKeyType string

const (
	// RequestPathCtxKey is the key used to store the request path in the request context
	RequestPathCtxKey RequestPathKeyType = "request_path"
)

// ParseURLPath parses the URL path from the echo context
func ParseURLPath(c echo.Context) (string, error) {
	parsedURL, err := url.Parse(c.Request().RequestURI)
	if err != nil {
		return "", err
	}

	return parsedURL.Path, nil
}

// GetURLPathFromContext returns the request path from the context
func GetURLPathFromContext(ctx context.Context) (string, error) {
	// Get request path for metrics
	requestPath, ok := ctx.Value(RequestPathCtxKey).(string)
	if !ok || (ok && len(requestPath) == 0) {
		requestPath = "unknown"
	}
	return requestPath, nil
}

// GetIsHumanDenomsQueryParam returns the value of the humanDenoms query parameter
// If the query parameter is not present, it returns false
// Errors if the value is not a valid boolean.
func GetIsHumanDenomsQueryParam(c echo.Context) (bool, error) {
	isHumanDenomsStr := c.QueryParam("humanDenoms")

	if len(isHumanDenomsStr) > 0 {
		return strconv.ParseBool(isHumanDenomsStr)
	}

	return false, nil
}
