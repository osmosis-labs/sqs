package types

import "errors"

// Handler Errors
var (
	ErrValidationFailed                = errors.New("validation failed")
	ErrTokenInNotValid                 = errors.New("tokenIn is invalid - must be in the format amountDenom")
	ErrTokenOutNotValid                = errors.New("tokenOut is invalid - must be in the format amountDenom")
	ErrTokenInDenomNotSpecified        = errors.New("tokenInDenom is required")
	ErrTokenOutDenomNotSpecified       = errors.New("tokenOutDenom is required")
	ErrTokenOutNotSpecified            = errors.New("tokenOut is required")
	ErrTokenInNotSpecified             = errors.New("tokenIn is required")
	ErrSwapMethodNotValid              = errors.New("swap method is invalid - must be either swap exact amount in or swap exact amount out")
	ErrPoolIDNotValid                  = errors.New("pool ID must be integer")
	ErrNumOfTokenOutDenomPoolsMismatch = errors.New("number of tokenOutDenom must be equal to number of pool IDs")
	ErrNumOfTokenInDenomPoolsMismatch  = errors.New("number of tokenInDenom must be equal to number of pool IDs")
	ErrInvalidRouteType                = errors.New("invalid route type")
)
