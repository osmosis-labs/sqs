package types

import (
	"errors"
	"strconv"
	"strings"

	"github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
)

// GetDirectCustomQuoteRequest represents
type GetDirectCustomQuoteRequest struct {
	TokenIn        *sdk.Coin
	TokenOutDenom  []string
	TokenOut       *sdk.Coin
	TokenInDenom   []string
	PoolID         []uint64 // list of the pool ID
	ApplyExponents bool     // Boolean flag indicating whether to apply exponents to the spot price. False by default.
}

// UnmarshalHTTPRequest unmarshals the HTTP request to GetDirectCustomQuoteRequest.
// It returns an error if the request is invalid.
func (r *GetDirectCustomQuoteRequest) UnmarshalHTTPRequest(c echo.Context) error {
	var err error
	r.ApplyExponents, err = http.ParseBooleanQueryParam(c, "applyExponents")
	if err != nil {
		return err
	}

	if tokenIn := c.QueryParam("tokenIn"); tokenIn != "" {
		tokenInCoin, err := sdk.ParseCoinNormalized(tokenIn)
		if err != nil {
			return ErrTokenInNotValid
		}
		r.TokenIn = &tokenInCoin
	}

	if tokenOut := c.QueryParam("tokenOut"); tokenOut != "" {
		tokenOutCoin, err := sdk.ParseCoinNormalized(tokenOut)
		if err != nil {
			return ErrTokenOutNotValid
		}
		r.TokenOut = &tokenOutCoin
	}

	r.TokenInDenom = strings.Split(c.QueryParam("tokenInDenom"), ",")
	r.TokenOutDenom = strings.Split(c.QueryParam("tokenOutDenom"), ",")

	// We accept two poolIDs and poolID parameters, and require at least one of them to be filled
	poolIDStr := strings.Split(c.QueryParam("poolID"), ",")
	if len(poolIDStr) == 0 {
		return errors.New("poolID is required")
	}

	for _, v := range poolIDStr {
		i, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return ErrPoolIDNotValid
		}
		r.PoolID = append(r.PoolID, i)
	}

	return nil
}

// SwapMethod returns the swap method of the request.
// Request may contain data for both swap methods, only one of them should be specified, otherwise it's invalid.
func (r *GetDirectCustomQuoteRequest) SwapMethod() domain.TokenSwapMethod {
	exactIn := r.TokenIn != nil && len(r.TokenOutDenom) > 0
	exactOut := r.TokenOut != nil && len(r.TokenInDenom) > 0

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
func (r *GetDirectCustomQuoteRequest) Validate() error {
	method := r.SwapMethod()
	if method == domain.TokenSwapMethodInvalid {
		return ErrSwapMethodNotValid
	}

	// Validate swap method exact amount in
	if method == domain.TokenSwapMethodExactIn {
		// one output per each pool
		if len(r.TokenOutDenom) != len(r.PoolID) {
			return ErrNumOfTokenOutDenomPoolsMismatch
		}

		// no duplicate denoms allowed
		for _, v := range r.TokenOutDenom {
			if err := domain.ValidateInputDenoms(r.TokenIn.Denom, v); err != nil {
				return err
			}
		}
	}

	// Validate swap method exact amount out
	if method == domain.TokenSwapMethodExactOut {
		// one output per each pool
		if len(r.TokenInDenom) != len(r.PoolID) {
			return ErrNumOfTokenInDenomPoolsMismatch
		}

		// no duplicate denoms allowed
		for _, v := range r.TokenInDenom {
			if err := domain.ValidateInputDenoms(r.TokenOut.Denom, v); err != nil {
				return err
			}
		}
	}

	return nil
}
