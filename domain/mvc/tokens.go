package mvc

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

// TokensUsecase defines an interface for the tokens usecase.
type TokensUsecase interface {
	// GetMetadataByChainDenom returns token metadata for a given chain denom.
	GetMetadataByChainDenom(ctx context.Context, denom string) (domain.Token, error)

	// GetFullTokenMetadata returns token metadata for all chain denoms as a map.
	GetFullTokenMetadata(ctx context.Context) (map[string]domain.Token, error)

	// GetChainDenom returns chain denom by human denom
	GetChainDenom(ctx context.Context, humanDenom string) (string, error)

	// GetChainScalingFactorByDenomMut returns a chain scaling factor for a given denom
	// and a boolean flag indicating whether the scaling factor was found or not.
	// Note that the returned decimal is a shared resource and must not be mutated.
	// A clone should be made for any mutative operation.
	GetChainScalingFactorByDenomMut(ctx context.Context, denom string) (osmomath.Dec, error)

	// GetSpotPriceScalingFactorByDenomMut returns the scaling factor for spot price.
	GetSpotPriceScalingFactorByDenom(ctx context.Context, baseDenom, quoteDenom string) (osmomath.Dec, error)

	// GetPrices returns prices for all given base and quote denoms given a pricing strategy or, otherwise, error, if any.
	// The options configure some customization with regards to how prices are computed.
	// By default, the prices are computes from chain as the source and going through caches
	// The outer map consists of base denoms as keys.
	// The inner map consists of quote denoms as keys.
	// The result of the inner map is prices of the outer base and inner quote.
	GetPrices(ctx context.Context, baseDenoms []string, quoteDenoms []string, opts ...domain.PricingOption) (map[string]map[string]any, error)

	// RegisterPricingStrategy registers a pricing strategy for a given pricing source.
	RegisterPricingStrategy(source domain.PricingSourceType, strategy domain.PricingSource)
}
