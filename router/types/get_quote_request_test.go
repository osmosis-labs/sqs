package types_test

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetQuoteRequestUnmarshal tests the UnmarshalHTTPRequest method of GetQuoteRequest.
func TestGetQuoteRequestUnmarshal(t *testing.T) {
	testcases := []struct {
		name           string
		queryParams    map[string]string
		expectedResult *types.GetQuoteRequest
		expectedError  bool
	}{
		{
			name: "valid request with tokenIn and tokenOut",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOutDenom":  "usdc",
				"tokenOut":       "1000usdc",
				"tokenInDenom":   "atom",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedResult: &types.GetQuoteRequest{
				TokenIn:        &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOutDenom:  "usdc",
				TokenOut:       &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom:   "atom",
				SingleRoute:    true,
				ApplyExponents: true,
			},
		},
		{
			name: "invalid singleRoute param",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOut":       "1000usdc",
				"singleRoute":    "invalid",
				"applyExponents": "true",
			},
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "invalid applyExponents param",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOut":       "1000usdc",
				"singleRoute":    "true",
				"applyExponents": "invalid",
			},
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "invalid tokenIn param",
			queryParams: map[string]string{
				"tokenIn":        "invalid_token",
				"tokenOut":       "1000usdc",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "invalid tokenOut param",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOut":       "invalid_token",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedResult: nil,
			expectedError:  true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(echo.GET, "/", nil)
			q := req.URL.Query()
			for k, v := range tc.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			var result types.GetQuoteRequest
			err := (&result).UnmarshalHTTPRequest(c)

			if tc.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// GetQuoteRequest must contain the expected result if the status is OK
			assert.Equal(t, tc.expectedResult, &result)
		})
	}
}

// TestGetQuoteRequestSwapMethod tests the SwapMethod method of GetQuoteRequest.
func TestGetQuoteRequestSwapMethod(t *testing.T) {
	testcases := []struct {
		name           string
		request        *types.GetQuoteRequest
		expectedMethod domain.TokenSwapMethod
	}{
		{
			name: "valid exact in swap method",
			request: &types.GetQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOutDenom: "usdc",
			},
			expectedMethod: domain.TokenSwapMethodExactIn,
		},
		{
			name: "valid exact out swap method",
			request: &types.GetQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom: "ust",
			},
			expectedMethod: domain.TokenSwapMethodExactOut,
		},
		{
			name: "invalid swap method with both tokenIn and tokenOut",
			request: &types.GetQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOut:      &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom:  "ust",
				TokenOutDenom: "usdc",
			},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
		{
			name: "invalid swap method with only tokenIn",
			request: &types.GetQuoteRequest{
				TokenIn: &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
			},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
		{
			name: "invalid swap method with only tokenOut",
			request: &types.GetQuoteRequest{
				TokenOut: &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
			},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
		{
			name:           "invalid swap method with neither tokenIn nor tokenOut",
			request:        &types.GetQuoteRequest{},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.request.SwapMethod()
			assert.Equal(t, tc.expectedMethod, method)
		})
	}
}

// TestGetQuoteRequestValidate tests the Validate method of GetQuoteRequest.
func TestGetQuoteRequestValidate(t *testing.T) {
	testcases := []struct {
		name          string
		request       *types.GetQuoteRequest
		expectedError error
	}{
		{
			name: "valid exact in request",
			request: &types.GetQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOutDenom: "usdc",
			},
			expectedError: nil,
		},
		{
			name: "valid exact out request",
			request: &types.GetQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom: "ust",
			},
			expectedError: nil,
		},
		{
			name: "invalid request with both tokenIn and tokenOut",
			request: &types.GetQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOut:      &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom:  "ust",
				TokenOutDenom: "usdc",
			},
			expectedError: types.ErrSwapMethodNotValid,
		},
		{
			name: "invalid exact in request with invalid denoms",
			request: &types.GetQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenOutDenom: "usdc",
			},
			expectedError: domain.SameDenomError{
				DenomA: "usdc",
				DenomB: "usdc",
			},
		},
		{
			name: "invalid exact out request with invalid denoms",
			request: &types.GetQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdt", Amount: osmomath.NewInt(1000)},
				TokenInDenom: "usdt",
			},
			expectedError: domain.SameDenomError{
				DenomA: "usdt",
				DenomB: "usdt",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestValidateSimulationParams(t *testing.T) {
	tests := []struct {
		name                 string
		swapMethod           domain.TokenSwapMethod
		simulatorAddress     string
		slippageToleranceStr string
		want                 osmomath.Dec

		expectedError bool
	}{
		{
			name:                 "valid simulation params",
			swapMethod:           domain.TokenSwapMethodExactIn,
			simulatorAddress:     "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			slippageToleranceStr: "0.01",
			want:                 osmomath.MustNewDecFromStr("0.01"),
		},
		{
			name:                 "exact out swap method",
			swapMethod:           domain.TokenSwapMethodExactOut,
			simulatorAddress:     "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			slippageToleranceStr: "0.01",
			want:                 osmomath.MustNewDecFromStr("0.01"),

			expectedError: true,
		},
		{
			name:                 "invalid simulator address",
			swapMethod:           domain.TokenSwapMethodExactIn,
			simulatorAddress:     "invalid",
			slippageToleranceStr: "0.01",
			expectedError:        true,
		},
		{
			name:                 "empty slippage tolerance",
			swapMethod:           domain.TokenSwapMethodExactIn,
			simulatorAddress:     "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			slippageToleranceStr: "",
			expectedError:        true,
		},
		{
			name:                 "invalid slippage tolerance",
			swapMethod:           domain.TokenSwapMethodExactIn,
			simulatorAddress:     "osmo13t8prr8hu7hkuksnfrd25vpvvnrfxr223k59ph",
			slippageToleranceStr: "invalid",
			expectedError:        true,
		},
		{
			name:                 "exact out with no simulator address or slippage tolerance",
			swapMethod:           domain.TokenSwapMethodExactOut,
			simulatorAddress:     "",
			slippageToleranceStr: "",
			want:                 osmomath.Dec{},
		},
		{
			name:                 "exact in with no simulator address or slippage tolerance",
			swapMethod:           domain.TokenSwapMethodExactIn,
			simulatorAddress:     "",
			slippageToleranceStr: "",
			want:                 osmomath.Dec{},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			got, err := types.ValidateSimulationParams(tt.swapMethod, tt.simulatorAddress, tt.slippageToleranceStr)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
