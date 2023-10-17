package usecase

import (
	"context"
	"time"

	"github.com/osmosis-labs/sqs/domain"
)

type quoteUsecase struct {
	contextTimeout time.Duration
}

// NewArticleUsecase will create new an articleUsecase object representation of domain.ArticleUsecase interface
func NewQuoteUsecase(timeout time.Duration) domain.QuoteUsecase {
	return &quoteUsecase{
		contextTimeout: timeout,
	}
}

// GetOutByTokenIn returns the amount out by token in in the given pool ID.
func (a *quoteUsecase) GetOutByTokenIn(ctx context.Context, poolID uint64, tokenIn string, tokenOutDenom string, swapFee string) (amount string, err error) {
	// ctx, cancel := context.WithTimeout(c, a.contextTimeout)
	// defer cancel()

	// g, ctx := errgroup.WithContext(c)
	return "", nil
}
