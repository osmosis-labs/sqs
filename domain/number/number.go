// Package number provides utility functions for working with numbers.
package number

import (
	"strconv"
	"strings"
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

// ParseNumberType parses a comma-separated list of numbers into a slice of the specified type.
func ParseNumberType[T any](numbersParam string, parseFn func(s string) (T, error)) ([]T, error) {
	numStrings := splitAndTrim(numbersParam, ",")

	var numbers []T
	for _, numStr := range numStrings {
		num, err := parseFn(numStr)
		if err != nil {
			return nil, err
		}
		numbers = append(numbers, num)
	}

	return numbers, nil
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
