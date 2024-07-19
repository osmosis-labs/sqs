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
func (r *GetQuoteRequest) SwapMethod() domain.TokenSwapMethod {
	if r.TokenIn != nil && r.TokenOutDenom != "" {
		return domain.TokenSwapMethodExactIn
	}

	if r.TokenOut != nil && r.TokenInDenom != "" {
		return domain.TokenSwapMethodExactOut
	}

	return domain.TokenSwapMethodInvalid
}

// IsSwapExactAmountIn returns true if the swap method is exact amount in.
func (r *GetQuoteRequest) IsSwapExactAmountIn() bool {
	return r.SwapMethod() == domain.TokenSwapMethodExactIn
}

// IsSwapExactAmountOut returns true if the swap method is exact amount out.
func (r *GetQuoteRequest) IsSwapExactAmountOut() bool {
	return r.SwapMethod() == domain.TokenSwapMethodExactOut
}

// Validate validates the GetQuoteRequest
func (r *GetQuoteRequest) Validate() error {
	// Request must have contain either swap exact amount in or swap exact amount out
	if (r.IsSwapExactAmountIn() && r.IsSwapExactAmountOut()) || (!r.IsSwapExactAmountIn() && !r.IsSwapExactAmountOut()) {
		return ErrSwapMethodNotValid
	}

	// Validate swap method exact amount in
	if r.IsSwapExactAmountIn() {
		if r.TokenIn == nil {
			return ErrTokenInNotSpecified
		}

		if r.TokenOutDenom == "" {
			return ErrTokenOutDenomNotSpecified
		}

		if err := domain.ValidateInputDenoms(r.TokenIn.Denom, r.TokenOutDenom); err != nil {
			return err
		}

		return nil
	}

	// Validate swap method exact amount out
	if r.IsSwapExactAmountOut() {
		if r.TokenOut == nil {
			return ErrTokenOutNotSpecified
		}

		if r.TokenInDenom == "" {
			return ErrTokenInDenomNotSpecified
		}

		if err := domain.ValidateInputDenoms(r.TokenOut.Denom, r.TokenInDenom); err != nil {
			return err
		}
	}

	return nil
}
