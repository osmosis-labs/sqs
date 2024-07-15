package http_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/labstack/echo/v4"
	routerdelivery "github.com/osmosis-labs/sqs/router/delivery/http"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

type RouterHandlerSuite struct {
	routertesting.RouterTestHelper
}

var (
	UOSMO = routertesting.UOSMO
	USDC  = routertesting.USDC
	UATOM = routertesting.ATOM
)

func TestRouterHandlerSuite(t *testing.T) {
	suite.Run(t, new(RouterHandlerSuite))
}

// TestGetPoolsValidTokenInTokensOut tests parsing pools, token in and token out parameters
// from the request.
func TestGetPoolsValidTokenInTokensOut(t *testing.T) {
	testCases := []struct {
		name string

		// input
		uri string

		// expected output
		tokenIn  string
		poolIDs  []uint64
		tokenOut []string

		err error
	}{
		{
			name:     "happy case - token through single pool",
			uri:      "http://localhost?tokenIn=10OSMO&poolID=1&tokenOutDenom=USDC",
			tokenIn:  "10OSMO",
			poolIDs:  []uint64{1},
			tokenOut: []string{"USDC"},
		},
		{
			name: "fail case - token through single pool",
			uri:  "http://localhost?tokenIn=&poolID=1&tokenOutDenom=USDC",
			err:  routerdelivery.ErrTokenInNotSpecified,
		},
		{
			name:     "happy case - token through multi pool",
			uri:      "http://localhost?tokenIn=56OSMO&poolID=1,5,7&tokenOutDenom=ATOM,AKT,USDC",
			tokenIn:  "56OSMO",
			poolIDs:  []uint64{1, 5, 7},
			tokenOut: []string{"ATOM", "AKT", "USDC"},
		},
		{
			name: "fail case - token through multi pool",
			uri:  "http://localhost?tokenIn=56OSMO&poolID=1,5&tokenOutDenom=ATOM,AKT,USDC",
			err:  routerdelivery.ErrNumOfTokenOutDenomPoolsMismatch,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := echo.New().NewContext(
				httptest.NewRequest(http.MethodGet, tc.uri, nil),
				nil,
			)

			poolIDs, tokenOut, tokenIn, err := routerdelivery.GetPoolsValidTokenInTokensOut(ctx)
			if !errors.Is(err, tc.err) {
				t.Fatalf("got %v, want %v", err, tc.err)
			}

			// on error output of the function is undefined
			if err != nil {
				t.SkipNow()
			}

			if slices.Compare(poolIDs, tc.poolIDs) != 0 {
				t.Fatalf("got %v, want %v", poolIDs, tc.poolIDs)
			}

			if slices.Compare(tokenOut, tc.tokenOut) != 0 {
				t.Fatalf("got %v, want %v", tokenOut, tc.tokenOut)
			}

			if tokenIn != tc.tokenIn {
				t.Fatalf("got %v, want %v", tokenIn, tc.tokenIn)
			}
		})
	}
}
