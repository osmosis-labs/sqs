package types

import (
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
)

// GetQuoteRequest represents swap quote request for the /router/quote endpoint.
type GetQuoteRequest struct {
	TokenIn                     *sdk.Coin
	TokenOutDenom               string
	TokenOut                    *sdk.Coin
	TokenInDenom                string
	SingleRoute                 bool
	SimulatorAddress            string
	SlippageToleranceMultiplier osmomath.Dec
	AppendBaseFee               bool
	HumanDenoms                 bool
	ApplyExponents              bool
}

// UnmarshalHTTPRequest unmarshals the HTTP request to GetQuoteRequest.
// It returns an error if the request is invalid.
func (r *GetQuoteRequest) UnmarshalHTTPRequest(c echo.Context) error {
	var err error
	r.SingleRoute, err = http.ParseBooleanQueryParam(c, "singleRoute")
	if err != nil {
		return err
	}

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

	r.TokenInDenom = c.QueryParam("tokenInDenom")
	r.TokenOutDenom = c.QueryParam("tokenOutDenom")

	simulatorAddress := c.QueryParam("simulatorAddress")
	slippageToleranceStr := c.QueryParam("simulationSlippageTolerance")

	slippageToleranceDec, err := validateSimulationParams(r.SwapMethod(), simulatorAddress, slippageToleranceStr)
	if err != nil {
		return err
	}

	r.SimulatorAddress = simulatorAddress
	r.SlippageToleranceMultiplier = slippageToleranceDec

	r.AppendBaseFee, err = http.ParseBooleanQueryParam(c, "appendBaseFee")
	if err != nil {
		return err
	}

	return nil
}

// validateSimulationParams validates the simulation parameters.
// Returns error if the simulation parameters are invalid.
// Returns slippage tolerance if it's valid.
func validateSimulationParams(swapMethod domain.TokenSwapMethod, simulatorAddress string, slippageToleranceStr string) (osmomath.Dec, error) {
	if simulatorAddress != "" {
		_, err := sdk.AccAddressFromBech32(simulatorAddress)
		if err != nil {
			return osmomath.Dec{}, fmt.Errorf("simulator address is not valid: (%s) (%w)", simulatorAddress, err)
		}

		// Validate that simulation is only requested for "out given in" swap method.
		if swapMethod != domain.TokenSwapMethodExactIn {
			return osmomath.Dec{}, fmt.Errorf("only 'out given in' swap method is supported for simulation")
		}

		if slippageToleranceStr == "" {
			return osmomath.Dec{}, fmt.Errorf("slippage tolerance is required for simulation")
		}

		slippageTolerance, err := osmomath.NewDecFromStr(slippageToleranceStr)
		if err != nil {
			return osmomath.Dec{}, fmt.Errorf("slippage tolerance is not valid: %w", err)
		}

		if slippageTolerance.LTE(osmomath.ZeroDec()) {
			return osmomath.Dec{}, fmt.Errorf("slippage tolerance must be greater than 0")
		}

		return slippageTolerance, nil
	} else {
		if slippageToleranceStr != "" {
			return osmomath.Dec{}, fmt.Errorf("slippage tolerance is not supported without simulator address")
		}
	}

	return osmomath.Dec{}, nil
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
