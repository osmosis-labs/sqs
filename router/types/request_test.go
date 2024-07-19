package types_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/router/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/stretchr/testify/assert"
)

func TestGetQuoteRequestUnmarhal(t *testing.T) {
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
				"tokenOut":       "1000usdc",
				"singleRoute":    "true",
				"applyExponents": "true",
			},
			expectedResult: &types.GetQuoteRequest{
				TokenIn:        &sdk.Coin{Denom: "ust", Amount: sdk.NewInt(1000)},
				TokenOut:       &sdk.Coin{Denom: "usdc", Amount: sdk.NewInt(1000)},
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
			expectedError:  types.ErrTokenNotValid,
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
			expectedError:  types.ErrTokenNotValid,
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


// TODO: test validate method
