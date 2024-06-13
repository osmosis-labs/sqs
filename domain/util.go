package domain

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// ParseNumbers parses a comma-separated list of numbers into a slice of unit64.
func ParseNumbers(numbersParam string) ([]uint64, error) {
	var numbers []uint64
	numStrings := splitAndTrim(numbersParam, ",")

	for _, numStr := range numStrings {
		num, err := strconv.ParseUint(numStr, 10, 64)
		if err != nil {
			return nil, err
		}
		numbers = append(numbers, num)
	}

	return numbers, nil
}

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

// ValidateInputDenoms returns nil of two denoms are valid, otherwise an error.
// This is to be used as a parameter validation for queries.
// For example, token in denom must not equal token out denom for quotes.
func ValidateInputDenoms(denomA, denomB string) error {
	if denomA == denomB {
		return SameDenomError{
			DenomA: denomA,
			DenomB: denomB,
		}
	}

	return nil
}

// splitAndTrim splits a string by a separator and trims the resulting strings.
func splitAndTrim(s, sep string) []string {
	var result []string
	for _, val := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
