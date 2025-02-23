package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTokensFromChainRegistry(t *testing.T) {
	tests := []struct {
		name             string
		responseBody     string
		expectedChecksum string
		expectedTokens   api.FeeTokens
		expectedError    bool
	}{
		{
			name: "Valid response with one token",
			responseBody: `{
				"$schema": "../chain.schema.json",
				"chain_name": "osmosis",
				"status": "live",
				"network_type": "mainnet",
				"website": "https://osmosis.zone/",
				"pretty_name": "Osmosis",
				"chain_type": "cosmos",
				"chain_id": "osmosis-1",
				"bech32_prefix": "osmo",
				"daemon_name": "osmosisd",
				"node_home": "$HOME/.osmosisd",
				"key_algos": [
					"secp256k1"
				],
				"slip44": 118,
				"fees": {
					"fee_tokens": [
						{
							"denom": "uosmo",
							"fixed_min_gas_price": 0.0025,
							"low_gas_price": 0.0025,
							"average_gas_price": 0.025,
							"high_gas_price": 0.04
						}
					]
				}
			}`,
			expectedTokens: api.FeeTokens{
				{
					Denom:            "uosmo",
					FixedMinGasPrice: 0.0025,
					LowGasPrice:      0.0025,
					AverageGasPrice:  0.025,
					HighGasPrice:     0.04,
				},
			},
			expectedChecksum: "cb3d1a12ad01326cecd5518d88b8d02b",
			expectedError:    false,
		},
		{
			name:           "Invalid JSON response",
			responseBody:   `{"invalid": "json"`,
			expectedTokens: nil,
			expectedError:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock HTTP server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			defer server.Close()

			// Call the function with the mock server URL
			tokens, checksum, err := getFeeTokensFromChainRegistry(context.TODO(), server.URL)

			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedChecksum, checksum)
				require.Equal(t, tc.expectedTokens, tokens)
			}
		})
	}
}

func TestCalculateFeeTokenMarketValue(t *testing.T) {
	tests := []struct {
		name             string
		gasFeeToken      *api.FeeToken
		unitPrice        osmomath.BigDec
		expectedFixedMin osmomath.BigDec
		expectedLow      osmomath.BigDec
		expectedAverage  osmomath.BigDec
		expectedHigh     osmomath.BigDec
		expectError      bool
	}{
		{
			name: "Normal case",
			gasFeeToken: &api.FeeToken{
				FixedMinGasPrice: 0.0025,
				LowGasPrice:      0.0025,
				AverageGasPrice:  0.025,
				HighGasPrice:     0.04,
			},
			unitPrice:        osmomath.MustNewBigDecFromStr("0.3436"),
			expectedFixedMin: osmomath.MustNewBigDecFromStr("0.000859"),
			expectedLow:      osmomath.MustNewBigDecFromStr("0.000859"),
			expectedAverage:  osmomath.MustNewBigDecFromStr("0.008590"),
			expectedHigh:     osmomath.MustNewBigDecFromStr("0.013744"),
			expectError:      false,
		},
		{
			name: "Zero prices",
			gasFeeToken: &api.FeeToken{
				FixedMinGasPrice: 0,
				LowGasPrice:      0,
				AverageGasPrice:  0,
				HighGasPrice:     0,
			},
			unitPrice:        osmomath.MustNewBigDecFromStr("3.5"),
			expectedFixedMin: osmomath.ZeroBigDec(),
			expectedLow:      osmomath.ZeroBigDec(),
			expectedAverage:  osmomath.ZeroBigDec(),
			expectedHigh:     osmomath.ZeroBigDec(),
			expectError:      false,
		},
		{
			name: "Very small prices",
			gasFeeToken: &api.FeeToken{
				FixedMinGasPrice: 0.000001,
				LowGasPrice:      0.000001,
				AverageGasPrice:  0.000001,
				HighGasPrice:     0.000001,
			},
			unitPrice:        osmomath.MustNewBigDecFromStr("3.5"),
			expectedFixedMin: osmomath.MustNewBigDecFromStr("0.0000035"),
			expectedLow:      osmomath.MustNewBigDecFromStr("0.0000035"),
			expectedAverage:  osmomath.MustNewBigDecFromStr("0.0000035"),
			expectedHigh:     osmomath.MustNewBigDecFromStr("0.0000035"),
			expectError:      false,
		},
		{
			name: "Very large prices",
			gasFeeToken: &api.FeeToken{
				FixedMinGasPrice: 1000000,
				LowGasPrice:      1000000,
				AverageGasPrice:  1000000,
				HighGasPrice:     1000000,
			},
			unitPrice:        osmomath.MustNewBigDecFromStr("3.5"),
			expectedFixedMin: osmomath.MustNewBigDecFromStr("3500000"),
			expectedLow:      osmomath.MustNewBigDecFromStr("3500000"),
			expectedAverage:  osmomath.MustNewBigDecFromStr("3500000"),
			expectedHigh:     osmomath.MustNewBigDecFromStr("3500000"),
			expectError:      false,
		},
		{
			name: "Zero unit price",
			gasFeeToken: &api.FeeToken{
				FixedMinGasPrice: 0.0025,
				LowGasPrice:      0.0025,
				AverageGasPrice:  0.025,
				HighGasPrice:     0.04,
			},
			unitPrice:        osmomath.ZeroBigDec(),
			expectedFixedMin: osmomath.ZeroBigDec(),
			expectedLow:      osmomath.ZeroBigDec(),
			expectedAverage:  osmomath.ZeroBigDec(),
			expectedHigh:     osmomath.ZeroBigDec(),
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			fixedMin, low, average, high, err := calculateFeeTokenMarketValue(ctx, tt.gasFeeToken, tt.unitPrice)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, tt.expectedFixedMin.Equal(fixedMin), "Fixed min market value mismatch. Expected %s, got %s", tt.expectedFixedMin, fixedMin)
				assert.True(t, tt.expectedLow.Equal(low), "Low market value mismatch. Expected %s, got %s", tt.expectedLow, low)
				assert.True(t, tt.expectedAverage.Equal(average), "Average market value mismatch. Expected %s, got %s", tt.expectedAverage, average)
				assert.True(t, tt.expectedHigh.Equal(high), "High market value mismatch. Expected %s, got %s", tt.expectedHigh, high)
			}
		})
	}
}

func TestCalculateTokenQuantity(t *testing.T) {
	tests := []struct {
		name           string
		amount         string
		unitPrice      string
		expectedResult string
	}{
		{
			name:           "Normal case",
			amount:         "0.1",
			unitPrice:      "0.6479",
			expectedResult: "0.154344806297268096928538354684364871",
		},
		{
			name:           "Zero amount",
			amount:         "0",
			unitPrice:      "2",
			expectedResult: "0",
		},
		{
			name:           "Zero unit price",
			amount:         "10",
			unitPrice:      "0",
			expectedResult: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			amount := osmomath.MustNewBigDecFromStr(tt.amount)
			unitPrice := osmomath.MustNewBigDecFromStr(tt.unitPrice)

			result := calculateTokenQuantity(ctx, amount, unitPrice)

			expected := osmomath.MustNewBigDecFromStr(tt.expectedResult)
			assert.True(t, expected.Equal(result), "Expected %s, but got %s", expected, result)
		})
	}
}
