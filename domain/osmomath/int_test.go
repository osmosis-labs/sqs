package osmomath_test

import (
	"math/big"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	sqsosmomath "github.com/osmosis-labs/sqs/domain/osmomath"
	"github.com/stretchr/testify/assert"
)

func TestSafeUint64(t *testing.T) {
	tests := []struct {
		name     string
		input    osmomath.Int
		expected uint64
	}{
		{
			name:     "Nil input",
			input:    osmomath.Int{},
			expected: 0,
		},
		{
			name:     "Zero input",
			input:    osmomath.NewInt(0),
			expected: 0,
		},
		{
			name:     "Negative input",
			input:    osmomath.NewInt(-5),
			expected: 0,
		},
		{
			name:     "Positive input within uint64 range",
			input:    osmomath.NewInt(42),
			expected: 42,
		},
		{
			name:     "Maximum uint64 value",
			input:    osmomath.NewIntFromUint64(^uint64(0)),
			expected: ^uint64(0),
		},
		{
			name:     "Value exceeding uint64 range",
			input:    osmomath.NewIntFromBigInt(new(big.Int).Add(big.NewInt(0).SetUint64(^uint64(0)), big.NewInt(1))),
			expected: ^uint64(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sqsosmomath.SafeUint64(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
