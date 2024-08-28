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
)

// TestGetDirectCustomQuoteRequestUnmarshal tests the UnmarshalHTTPRequest method of GetDirectCustomQuoteRequest.
func TestGetDirectCustomQuoteRequestUnmarshal(t *testing.T) {
	testcases := []struct {
		name           string
		queryParams    map[string]string
		expectedResult *types.GetDirectCustomQuoteRequest
		expectedError  bool
	}{
		{
			name: "valid request with tokenIn and tokenOut",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOutDenom":  "usdc,ion",
				"tokenOut":       "1000usdc",
				"tokenInDenom":   "atom,uosmo",
				"poolID":         "1,23",
				"applyExponents": "true",
			},
			expectedResult: &types.GetDirectCustomQuoteRequest{
				TokenIn:        &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOutDenom:  []string{"usdc", "ion"},
				TokenOut:       &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom:   []string{"atom", "uosmo"},
				PoolID:         []uint64{1, 23},
				ApplyExponents: true,
			},
		},
		{
			name: "invalid poolID param",
			queryParams: map[string]string{
				"tokenIn":     "1000ust",
				"tokenOut":    "1000usdc",
				"singleRoute": "true",
				"poolID":      "invalid,10",
			},
			expectedError: true,
		},
		{
			name: "invalid applyExponents param",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOut":       "1000usdc",
				"singleRoute":    "true",
				"applyExponents": "invalid",
			},
			expectedError: true,
		},
		{
			name: "invalid tokenIn param",
			queryParams: map[string]string{
				"tokenIn":        "invalid_token",
				"tokenOut":       "1000usdc",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedError: true,
		},
		{
			name: "invalid tokenOut param",
			queryParams: map[string]string{
				"tokenIn":        "1000ust",
				"tokenOut":       "invalid_token",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedError: true,
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

			var result types.GetDirectCustomQuoteRequest
			err := (&result).UnmarshalHTTPRequest(c)

			if tc.expectedError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

// TestGetDirectCustomQuoteRequestSwapMethod tests the SwapMethod method of GetDirectCustomQuoteRequest.
func TestGetDirectCustomQuoteRequestSwapMethod(t *testing.T) {
	testcases := []struct {
		name           string
		request        *types.GetDirectCustomQuoteRequest
		expectedMethod domain.TokenSwapMethod
	}{
		{
			name: "valid exact in swap method",
			request: &types.GetDirectCustomQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOutDenom: []string{"usdc"},
				PoolID:        []uint64{1},
			},
			expectedMethod: domain.TokenSwapMethodExactIn,
		},
		{
			name: "valid exact out swap method",
			request: &types.GetDirectCustomQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom: []string{"ust"},
				PoolID:       []uint64{1},
			},
			expectedMethod: domain.TokenSwapMethodExactOut,
		},
		{
			name: "invalid swap method with both tokenIn and tokenOut",
			request: &types.GetDirectCustomQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOut:      &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom:  []string{"ust"},
				TokenOutDenom: []string{"usdc"},
				PoolID:        []uint64{1},
			},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
		{
			name: "invalid swap method with only tokenIn",
			request: &types.GetDirectCustomQuoteRequest{
				TokenIn: &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
			},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
		{
			name: "invalid swap method with only tokenOut",
			request: &types.GetDirectCustomQuoteRequest{
				TokenOut: &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
			},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
		{
			name:           "invalid swap method with neither tokenIn nor tokenOut",
			request:        &types.GetDirectCustomQuoteRequest{},
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

// TestGetDirectCustomQuoteRequestValidate tests the Validate method of GetDirectCustomQuoteRequest.
func TestGetDirectCustomQuoteRequestValidate(t *testing.T) {
	testcases := []struct {
		name          string
		request       *types.GetDirectCustomQuoteRequest
		expectedError error
	}{
		{
			name: "valid exact in request",
			request: &types.GetDirectCustomQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOutDenom: []string{"usdc"},
				PoolID:        []uint64{1},
			},
			expectedError: nil,
		},
		{
			name: "exact in request pool id and token out denom mismatch",
			request: &types.GetDirectCustomQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOutDenom: []string{"usdc", "usdt", "uusd"},
				PoolID:        []uint64{1, 2},
			},
			expectedError: types.ErrNumOfTokenOutDenomPoolsMismatch,
		},
		{
			name: "valid exact out request",
			request: &types.GetDirectCustomQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom: []string{"ust"},
				PoolID:       []uint64{1},
			},
			expectedError: nil,
		},
		{
			name: "exact out request pool id and token in denom mismatch",
			request: &types.GetDirectCustomQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom: []string{"usdc", "usdt", "uusd"},
				PoolID:       []uint64{1},
			},
			expectedError: types.ErrNumOfTokenInDenomPoolsMismatch,
		},
		{
			name: "invalid request: contains both tokenIn and tokenOut",
			request: &types.GetDirectCustomQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: osmomath.NewInt(1000)},
				TokenOut:      &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenInDenom:  []string{"ust"},
				TokenOutDenom: []string{"usdc"},
			},
			expectedError: types.ErrSwapMethodNotValid,
		},
		{
			name: "invalid exact in request with invalid denoms",
			request: &types.GetDirectCustomQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "usdc", Amount: osmomath.NewInt(1000)},
				TokenOutDenom: []string{"usdc"},
				PoolID:        []uint64{1},
			},
			expectedError: domain.SameDenomError{
				DenomA: "usdc",
				DenomB: "usdc",
			},
		},
		{
			name: "invalid exact out request with invalid denoms",
			request: &types.GetDirectCustomQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdt", Amount: osmomath.NewInt(1000)},
				TokenInDenom: []string{"usdt"},
				PoolID:       []uint64{1},
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
