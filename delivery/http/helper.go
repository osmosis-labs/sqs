package http

import (
	"strconv"

	"github.com/labstack/echo/v4"
)

// ParseBooleanQueryParam parses a boolean query parameter.
// Returns false if the parameter is not present.
// Errors if the value is not a valid boolean.
// Returns the boolean value and an error if any.
func ParseBooleanQueryParam(c echo.Context, paramName string) (paramValue bool, err error) {
	paramValueStr := c.QueryParam(paramName)
	if paramValueStr != "" {
		paramValue, err = strconv.ParseBool(paramValueStr)
		if err != nil {
			return false, err
		}
	}

	return paramValue, nil
}
