package usecase

import (
	"context"

	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"
)

type chainregistryUseCase struct {
}

// NewChainregistryUseCase creates a new chainregistry use case.
func NewChainregistryUseCase() *chainregistryUseCase {
	return &chainregistryUseCase{}
}

// GetFeeTokens implements mvc.ChainregistryUsecase.
func (p *chainregistryUseCase) GetFeeTokens(ctx context.Context) ([]*api.FeeToken, error) {
	return nil, nil
}
