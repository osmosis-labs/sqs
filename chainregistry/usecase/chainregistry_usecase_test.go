package usecase

import (
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"

	"github.com/stretchr/testify/require"
)

func TestGetTokensFromChainRegistry(t *testing.T) {
	tests := []struct {
		name             string
		responseBody     string
		expectedChecksum string
		expectedTokens   []*api.FeeToken
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
			expectedTokens: []*api.FeeToken{
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
			tokens, checksum, err := GetFeeTokensFromChainRegistry(server.URL)

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
