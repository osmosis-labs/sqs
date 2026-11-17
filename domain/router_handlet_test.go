package domain_test

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/number"
	"github.com/stretchr/testify/require"
)

// TestParseNumbers tests parsing a string of numbers to a slice of uint64
func TestParseNumbers(t *testing.T) {
	testCases := []struct {
		input           string
		expectedNumbers []uint64
		expectedError   bool
	}{
		{"", nil, false},                          // Empty string, expecting an empty slice and no error
		{"1,2,3", []uint64{1, 2, 3}, false},       // Comma-separated numbers, expecting slice {1, 2, 3} and no error
		{"42", []uint64{42}, false},               // Single number, expecting slice {42} and no error
		{"10,20,30", []uint64{10, 20, 30}, false}, // Another set of numbers

		// Add more test cases as needed
		{"abc", nil, true}, // Invalid input, expecting an error
	}

	for _, testCase := range testCases {
		actualNumbers, actualError := number.ParseNumbers(testCase.input)

		if testCase.expectedError {
			require.Error(t, actualError)
			return
		}

		// Check if the actual output matches the expected output
		if !reflect.DeepEqual(actualNumbers, testCase.expectedNumbers) {
			t.Errorf("Input: %s, Expected Numbers: %v, Actual Numbers: %v",
				testCase.input, testCase.expectedNumbers, actualNumbers)
		}
	}
}

// TestGetStatusCode ensures request errors map to 4xx, including when wrapped, and unknown errors stay 500.
func TestGetStatusCode(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected int
	}{
		{"nil", nil, http.StatusOK},
		{"pool not found", domain.PoolNotFoundError{PoolID: 1}, http.StatusNotFound},
		{"no candidate routes, wrapped", fmt.Errorf("%w, tokenOutDenom (uion)", domain.ErrNoCandidateRoutes), http.StatusNotFound},
		{"transmuter insufficient balance, wrapped", fmt.Errorf("%w, tokenOutDenom (uion)", domain.TransmuterInsufficientBalanceError{Denom: "uion"}), http.StatusUnprocessableEntity},
		{"desired output not producible", domain.ErrDesiredOutputNotProducible, http.StatusUnprocessableEntity},
		{"no tokens out", domain.ErrNoTokensOut, http.StatusBadRequest},
		{"unknown error", errors.New("boom"), http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, domain.GetStatusCode(tc.err))
		})
	}
}
