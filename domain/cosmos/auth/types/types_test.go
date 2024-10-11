package types_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAccount(t *testing.T) {
	tests := []struct {
		name           string
		address        string
		mockResponse   string
		expectedResult *authtypes.QueryAccountResponse
		expectedError  bool
	}{
		{
			name:    "Valid account",
			address: "cosmos1abcde",
			mockResponse: `{
				"account": {
					"sequence": "10",
					"account_number": "100"
				}
			}`,
			expectedResult: &authtypes.QueryAccountResponse{
				Account: authtypes.Account{
					Sequence:      10,
					AccountNumber: 100,
				},
			},
			expectedError: false,
		},
		{
			name:    "Invalid JSON response",
			address: "cosmos1fghij",
			mockResponse: `{
				"account": {
					"sequence": "invalid",
					"account_number": "100"
				}
			}`,
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name:           "Empty response",
			address:        "cosmos1klmno",
			mockResponse:   `{}`,
			expectedResult: nil,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.mockResponse))
			}))
			defer server.Close()

			client := authtypes.NewQueryClient(server.URL)
			result, err := client.GetAccount(context.Background(), tt.address)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}
