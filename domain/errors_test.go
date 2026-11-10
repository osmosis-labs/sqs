package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
)

// Validates that the pool type is formatted as a number. Converting the integer to a
// string directly yields the character with that code point instead.
func TestInvalidPoolTypeError(t *testing.T) {
	require.Equal(t, "invalid pool type: 3", domain.InvalidPoolTypeError{PoolType: 3}.Error())
}
