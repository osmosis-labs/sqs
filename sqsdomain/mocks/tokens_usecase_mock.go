package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
)

type TokensUseCaseMock struct {
	tokenPrecisionMap map[string]int
}

// GetDenomPrecisions implements domain.TokensUsecase.
func (tu *TokensUseCaseMock) GetDenomPrecisions(ctx context.Context) (map[string]int, error) {
	return tu.tokenPrecisionMap, nil
}

var _ domain.TokensUsecase = &TokensUseCaseMock{}
