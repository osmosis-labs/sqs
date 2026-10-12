package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFeeTokens(t *testing.T) {
	tests := []struct {
		name           string
		setupUseCase   func() *chainregistryUseCase
		ctx            context.Context
		expectedTokens []*api.FeeToken
		expectedError  bool
	}{
		{
			name: "Success - Returns tokens",
			setupUseCase: func() *chainregistryUseCase {
				uc := &chainregistryUseCase{
					command: make(chan cmd),
					tokens: []*api.FeeToken{
						{Denom: "uosmo", FixedMinGasPrice: 0.0025},
						{Denom: "uatom", FixedMinGasPrice: 0.0015},
					},
					logger: &log.NoOpLogger{},
				}
				go uc.run()
				return uc
			},
			ctx: context.Background(),
			expectedTokens: []*api.FeeToken{
				{Denom: "uosmo", FixedMinGasPrice: 0.0025},
				{Denom: "uatom", FixedMinGasPrice: 0.0015},
			},
			expectedError: false,
		},
		{
			name: "Success - Empty tokens",
			setupUseCase: func() *chainregistryUseCase {
				uc := &chainregistryUseCase{
					command: make(chan cmd),
					tokens:  []*api.FeeToken{},
					logger:  &log.NoOpLogger{},
				}
				go uc.run()
				return uc
			},
			ctx:            context.Background(),
			expectedTokens: []*api.FeeToken{},
			expectedError:  false,
		},
		{
			name: "Failure - Context cancelled",
			setupUseCase: func() *chainregistryUseCase {
				uc := &chainregistryUseCase{
					command: make(chan cmd),
					tokens: []*api.FeeToken{
						{Denom: "uosmo", FixedMinGasPrice: 0.0025},
					},
					logger: &log.NoOpLogger{},
				}
				go uc.run()
				return uc
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx
			}(),
			expectedTokens: nil,
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := tt.setupUseCase()

			tokens, err := uc.GetFeeTokens(tt.ctx)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTokens, tokens)
			}

			// Clean up
			close(uc.command)
		})
	}
}

func TestGetFeeTokensTimeout(t *testing.T) {
	uc := &chainregistryUseCase{
		command: make(chan cmd),
		tokens: []*api.FeeToken{
			{Denom: "uosmo", FixedMinGasPrice: 0.0025},
		},
		logger: &log.NoOpLogger{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := uc.GetFeeTokens(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "context deadline exceeded")

	// Clean up
	close(uc.command)
}

func TestProcessFeeTokens(t *testing.T) {
	tests := []struct {
		name           string
		inputTokens    api.FeeTokens
		mockPrices     domain.PricesResult
		expectedTokens api.FeeTokens
		expectedError  string
	}{
		{
			name: "Success - Process multiple tokens",
			inputTokens: api.FeeTokens{
				{Denom: "uosmo", FixedMinGasPrice: 0.0025, LowGasPrice: 0.0025, AverageGasPrice: 0.0025, HighGasPrice: 0.0025},
				{Denom: "uatom", FixedMinGasPrice: 0.0, LowGasPrice: 0.0, AverageGasPrice: 0.0, HighGasPrice: 0.0},
			},
			mockPrices: domain.PricesResult{
				"uosmo": {"factory/osmo147h5x9pcj7lm0cttlaefx6sqq5vdfnmwfcqxkmjd7exqm9gc7grqhr75m0/alloyed/allUSDC": domain.NewPriceResult(osmomath.MustNewBigDecFromStr("1.0"), nil)},
				"uatom": {"factory/osmo147h5x9pcj7lm0cttlaefx6sqq5vdfnmwfcqxkmjd7exqm9gc7grqhr75m0/alloyed/allUSDC": domain.NewPriceResult(osmomath.MustNewBigDecFromStr("2.0"), nil)},
			},
			expectedTokens: api.FeeTokens{
				{Denom: "uosmo", FixedMinGasPrice: 0.0025, LowGasPrice: 0.0025, AverageGasPrice: 0.0025, HighGasPrice: 0.0025},
				{Denom: "uatom", FixedMinGasPrice: 0.00125, LowGasPrice: 0.00125, AverageGasPrice: 0.00125, HighGasPrice: 0.00125},
			},
			expectedError: "",
		},
		{
			name:           "Empty input tokens",
			inputTokens:    api.FeeTokens{},
			mockPrices:     domain.PricesResult{},
			expectedTokens: nil,
			expectedError:  "",
		},
		{
			name: "Missing base denom price",
			inputTokens: api.FeeTokens{
				{Denom: "uosmo", FixedMinGasPrice: 0.0025, LowGasPrice: 0.0025, AverageGasPrice: 0.0025, HighGasPrice: 0.0025},
			},
			mockPrices: domain.PricesResult{
				"uosmo": {},
			},
			expectedTokens: nil,
			expectedError:  "failed to get price for uosmo/factory/osmo147h5x9pcj7lm0cttlaefx6sqq5vdfnmwfcqxkmjd7exqm9gc7grqhr75m0/alloyed/allUSDC",
		},
		{
			name: "Missing token price",
			inputTokens: api.FeeTokens{
				{Denom: "uosmo", FixedMinGasPrice: 0.0025, LowGasPrice: 0.0025, AverageGasPrice: 0.0025, HighGasPrice: 0.0025},
				{Denom: "uatom", FixedMinGasPrice: 0.0, LowGasPrice: 0.0, AverageGasPrice: 0.0, HighGasPrice: 0.0},
			},
			mockPrices: domain.PricesResult{
				"uosmo": {
					"factory/osmo147h5x9pcj7lm0cttlaefx6sqq5vdfnmwfcqxkmjd7exqm9gc7grqhr75m0/alloyed/allUSDC": domain.PriceResult{
						Price: osmomath.MustNewBigDecFromStr("1.0"),
					},
				},
				"uatom": {},
			},
			expectedTokens: nil,
			expectedError:  "failed to get price for uatom/factory/osmo147h5x9pcj7lm0cttlaefx6sqq5vdfnmwfcqxkmjd7exqm9gc7grqhr75m0/alloyed/allUSDC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockTokensUsecase := &mocks.TokensUsecaseMock{
				GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
					return tt.mockPrices, nil
				},
			}

			uc := &chainregistryUseCase{
				tokensUseCase: mockTokensUsecase,
				baseDenom:     "uosmo",
				quoteDenom:    "factory/osmo147h5x9pcj7lm0cttlaefx6sqq5vdfnmwfcqxkmjd7exqm9gc7grqhr75m0/alloyed/allUSDC",
				logger:        &log.NoOpLogger{},
			}

			result, err := uc.processFeeTokens(context.Background(), tt.inputTokens)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedTokens, result)
			}
		})
	}
}

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

func TestCalculateMarketValue(t *testing.T) {
	tests := []struct {
		name          string
		tokenQuantity float64
		unitPrice     string
		expected      string
		expectedError bool
	}{
		{
			name:          "Simple calculation",
			tokenQuantity: 100,
			unitPrice:     "2.5",
			expected:      "250",
			expectedError: false,
		},
		{
			name:          "Zero token quantity",
			tokenQuantity: 0,
			unitPrice:     "1.5",
			expected:      "0",
			expectedError: false,
		},
		{
			name:          "Zero unit price",
			tokenQuantity: 100,
			unitPrice:     "0",
			expected:      "0",
			expectedError: false,
		},
		{
			name:          "Large numbers",
			tokenQuantity: 1000000,
			unitPrice:     "1000000",
			expected:      "1000000000000",
			expectedError: false,
		},
		{
			name:          "Small numbers",
			tokenQuantity: 0.0001,
			unitPrice:     "0.0001",
			expected:      "0.00000001",
			expectedError: false,
		},
		{
			name:          "Invalid token quantity",
			tokenQuantity: -100,
			unitPrice:     "1.5",
			expected:      "-150",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			unitPrice, err := osmomath.NewBigDecFromStr(tt.unitPrice)
			require.NoError(t, err)

			result, err := calculateMarketValue(ctx, tt.tokenQuantity, unitPrice)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				expected, err := osmomath.NewBigDecFromStr(tt.expected)
				require.NoError(t, err)
				assert.True(t, result.Equal(expected), "Expected %s, got %s", expected, result)
			}
		})
	}
}
