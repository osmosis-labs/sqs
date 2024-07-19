package types

import (
	"github.com/osmosis-labs/sqs/domain"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
)

// GetQuoteRequest represents swap quote request for the /router/quote endpoint.
type GetQuoteRequest struct {
	TokenIn        *sdk.Coin
	TokenOutDenom  string
	TokenOut       *sdk.Coin
	TokenInDenom   string
	SingleRoute    bool
	HumanDenoms    bool
	ApplyExponents bool
}

// UnmarshalHTTPRequest unmarshals the HTTP request to GetQuoteRequest.
// It returns an error if the request is invalid.
// NOTE: Currently method for some cases returns an error, while for others
// it returns a response error. This is not consistent and should be fixed.
func (r *GetQuoteRequest) UnmarshalHTTPRequest(c echo.Context) error {
	var err error
	r.SingleRoute, err = domain.ParseBooleanQueryParam(c, "singleRoute")
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	r.ApplyExponents, err = domain.ParseBooleanQueryParam(c, "applyExponents")
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	if tokenIn := c.QueryParam("tokenIn"); tokenIn != "" {
		tokenInCoin, err := sdk.ParseCoinNormalized(tokenIn)
		if err != nil {
			return ErrTokenNotValid
		}
		r.TokenIn = &tokenInCoin
	}

	if tokenOut := c.QueryParam("tokenOut"); tokenOut != "" {
		tokenOutCoin, err := sdk.ParseCoinNormalized(tokenOut)
		if err != nil {
			return ErrTokenNotValid
		}
		r.TokenOut = &tokenOutCoin
	}

	r.TokenInDenom = c.QueryParam("tokenInDenom")
	r.TokenOutDenom = c.QueryParam("tokenOutDenom")

	return nil
}

// SwapMethod returns the swap method of the request.
// Request may contain data for both swap methods, only one of them should be specified, otherwise it's invalid.
func (r *GetQuoteRequest) SwapMethod() domain.TokenSwapMethod {
	exactIn := r.TokenIn != nil && r.TokenOutDenom != ""
	exactOut := r.TokenOut != nil && r.TokenInDenom != ""

	if exactIn && exactOut {
		return domain.TokenSwapMethodInvalid
	}

	if exactIn {
		return domain.TokenSwapMethodExactIn
	}

	if exactOut {
		return domain.TokenSwapMethodExactOut
	}

	return domain.TokenSwapMethodInvalid
}

// Validate validates the GetQuoteRequest.
func (r *GetQuoteRequest) Validate() error {
	method := r.SwapMethod()
	if method == domain.TokenSwapMethodInvalid {
		return ErrSwapMethodNotValid
	}

	// token denoms
	var a, b string

	// Validate swap method exact amount in
	if method == domain.TokenSwapMethodExactIn {
		a, b = r.TokenIn.Denom, r.TokenOutDenom
	}

	// Validate swap method exact amount out
	if method == domain.TokenSwapMethodExactOut {
		a, b = r.TokenOut.Denom, r.TokenInDenom
	}

	return domain.ValidateInputDenoms(a, b)
}
