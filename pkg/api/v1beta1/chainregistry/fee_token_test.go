package chainregistry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeeTokens_GetByDenom(t *testing.T) {
	// Setup test data
	feeTokens := FeeTokens{
		{Denom: "uosmo", FixedMinGasPrice: 0.0025},
		{Denom: "uatom", FixedMinGasPrice: 0.003},
		{Denom: "uusdc", FixedMinGasPrice: 0.002},
	}

	tests := []struct {
		name           string
		denom          string
		expectedToken  *FeeToken
		expectedExists bool
	}{
		{
			name:           "Existing denom uosmo",
			denom:          "uosmo",
			expectedToken:  &FeeToken{Denom: "uosmo", FixedMinGasPrice: 0.0025},
			expectedExists: true,
		},
		{
			name:           "Existing denom uatom",
			denom:          "uatom",
			expectedToken:  &FeeToken{Denom: "uatom", FixedMinGasPrice: 0.003},
			expectedExists: true,
		},
		{
			name:           "Existing denom uusdc",
			denom:          "uusdc",
			expectedToken:  &FeeToken{Denom: "uusdc", FixedMinGasPrice: 0.002},
			expectedExists: true,
		},
		{
			name:           "Non-existing denom",
			denom:          "unonexistent",
			expectedToken:  nil,
			expectedExists: false,
		},
		{
			name:           "Empty denom",
			denom:          "",
			expectedToken:  nil,
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := feeTokens.GetByDenom(tt.denom)

			if tt.expectedExists {
				assert.NotNil(t, result, "Expected to find a token")
				assert.Equal(t, tt.expectedToken, result, "Token mismatch")
			} else {
				assert.Nil(t, result, "Expected not to find a token")
			}
		})
	}
}
