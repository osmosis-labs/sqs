package types_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/stretchr/testify/assert"
)

// TestGetQuoteRequestUnmarshal tests the UnmarshalHTTPRequest method of GetQuoteRequest.
func TestGetQuoteRequestUnmarshal(t *testing.T) {
	testcases := []struct {
		name           string
		queryParams    map[string]string
		expectedResult *types.GetQuoteRequest
		expectedError  error
		expectedStatus int
		expectedBody   string
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
				TokenIn:        &sdk.Coin{Denom: "ust", Amount: sdk.NewInt(1000)},
				TokenOutDenom:  "usdc",
				TokenOut:       &sdk.Coin{Denom: "usdc", Amount: sdk.NewInt(1000)},
				TokenInDenom:   "atom",
				SingleRoute:    true,
				ApplyExponents: true,
			},
			expectedError:  nil,
			expectedStatus: http.StatusOK,
			expectedBody:   "",
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
			expectedError:  nil,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"message":"strconv.ParseBool: parsing \"invalid\": invalid syntax"}`,
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
			expectedError:  nil,
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"message":"strconv.ParseBool: parsing \"invalid\": invalid syntax"}`,
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
			expectedError:  types.ErrTokenInNotValid,
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
			expectedError:  types.ErrTokenOutNotValid,
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

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tc.expectedError, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expectedStatus, rec.Code)
			assert.Equal(t, tc.expectedBody, strings.TrimSpace(rec.Body.String())) // JSONEq fails

			// GetQuoteRequest must contain the expected result if the status is OK
			if tc.expectedStatus == http.StatusOK {
				assert.Equal(t, tc.expectedResult, &result)
			}
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
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: sdk.NewInt(1000)},
				TokenOutDenom: "usdc",
			},
			expectedMethod: domain.TokenSwapMethodExactIn,
		},
		{
			name: "valid exact out swap method",
			request: &types.GetQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdc", Amount: sdk.NewInt(1000)},
				TokenInDenom: "ust",
			},
			expectedMethod: domain.TokenSwapMethodExactOut,
		},
		{
			name: "invalid swap method with both tokenIn and tokenOut",
			request: &types.GetQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: sdk.NewInt(1000)},
				TokenOut:      &sdk.Coin{Denom: "usdc", Amount: sdk.NewInt(1000)},
				TokenInDenom:  "ust",
				TokenOutDenom: "usdc",
			},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
		{
			name: "invalid swap method with only tokenIn",
			request: &types.GetQuoteRequest{
				TokenIn: &sdk.Coin{Denom: "ust", Amount: sdk.NewInt(1000)},
			},
			expectedMethod: domain.TokenSwapMethodInvalid,
		},
		{
			name: "invalid swap method with only tokenOut",
			request: &types.GetQuoteRequest{
				TokenOut: &sdk.Coin{Denom: "usdc", Amount: sdk.NewInt(1000)},
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
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: sdk.NewInt(1000)},
				TokenOutDenom: "usdc",
			},
			expectedError: nil,
		},
		{
			name: "valid exact out request",
			request: &types.GetQuoteRequest{
				TokenOut:     &sdk.Coin{Denom: "usdc", Amount: sdk.NewInt(1000)},
				TokenInDenom: "ust",
			},
			expectedError: nil,
		},
		{
			name: "invalid request with both tokenIn and tokenOut",
			request: &types.GetQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "ust", Amount: sdk.NewInt(1000)},
				TokenOut:      &sdk.Coin{Denom: "usdc", Amount: sdk.NewInt(1000)},
				TokenInDenom:  "ust",
				TokenOutDenom: "usdc",
			},
			expectedError: types.ErrSwapMethodNotValid,
		},
		{
			name: "invalid exact in request with invalid denoms",
			request: &types.GetQuoteRequest{
				TokenIn:       &sdk.Coin{Denom: "usdc", Amount: sdk.NewInt(1000)},
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
				TokenOut:     &sdk.Coin{Denom: "usdt", Amount: sdk.NewInt(1000)},
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
