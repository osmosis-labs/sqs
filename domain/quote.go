package domain

import "context"

// Quote is representing the quote data struct
type Quote struct {
	PoolID   uint64 `json:"pool_id" validate:"required"`
	TokenIn  string `json:"token_in" validate:"required"`
	TokenOut string `json:"token_out" validate:"required"`
	Amount   string `json:"amount" validate:"required"`
}

// QuoteUsecase represent the quote's usecases
type QuoteUsecase interface {
	// GetLimitAmountByTokenIn(tokenInDenom string)
	// tokenIn: Toke
	// tokenOutDenom
	// swapFee?: Dec
	GetOutByTokenIn(ctx context.Context, poolID uint64, tokenIn string, tokenOutDenom string, swapFee string) (amount string, err error)

	// GetInByTokenOut()
}

// QuoteRepository represent the quote's repository contract
// type QuoteRepository interface {
// }
