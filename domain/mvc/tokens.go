package mvc

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
)

// TokensUsecase defines an interface for the tokens usecase.
type TokensUsecase interface {
	// GetMetadataByChainDenom returns token metadata for a given chain denom.
	GetMetadataByChainDenom(ctx context.Context, denom string) (domain.Token, error)

	// GetChainDenom returns chain denom by human denom
	GetChainDenom(ctx context.Context, humanDenom string) (string, error)

	// GetDenomPrecisions returns a map of all denom precisions.
	GetDenomPrecisions(ctx context.Context) (map[string]int, error)
}
