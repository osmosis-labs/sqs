package usecase

import (
	"context"
	"time"

	"github.com/osmosis-labs/router/domain"
)

type poolsUseCase struct {
	contextTimeout time.Duration
}

// NewPoolsUsecase will create a new pools use case object
func NewPoolsUsecase(timeout time.Duration) domain.PoolsUsecase {
	return &poolsUseCase{
		contextTimeout: timeout,
	}
}

// GetAllPools returns all pools from the repository.
func (a *poolsUseCase) GetAllPools(ctx context.Context, poolID uint64, tokenIn string, tokenOutDenom string, swapFee string) (amount string, err error) {
	// ctx, cancel := context.WithTimeout(c, a.contextTimeout)
	// defer cancel()

	// g, ctx := errgroup.WithContext(c)
	return "", nil
}
