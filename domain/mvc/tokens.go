package mvc

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

type PricingStrategy int

const (
	ChainPricing PricingStrategy = iota
	CoinGeckoPricing
)

// TokensUsecase defines an interface for the tokens usecase.
type TokensUsecase interface {
	// GetMetadataByChainDenom returns token metadata for a given chain denom.
	GetMetadataByChainDenom(ctx context.Context, denom string) (domain.Token, error)

	// GetFullTokenMetadata returns token metadata for all chain denoms as a map.
	GetFullTokenMetadata(ctx context.Context) (map[string]domain.Token, error)

	// GetChainDenom returns chain denom by human denom
	GetChainDenom(ctx context.Context, humanDenom string) (string, error)

	// GetDenomPrecisions returns a map of all denom precisions.
	GetDenomPrecisions(ctx context.Context) (map[string]int, error)

	// GetChainScalingFactorMut returns a chain scaling factor for a given precision
	// and a boolean flag indicating whether the scaling factor was found or not.
	// Note that the returned decimal is a shared resource and must not be mutated.
	// A clone should be made for any mutative operation.
	GetChainScalingFactorMut(precision int) (osmomath.Dec, bool)

	// GetPricingStrategy()
}
