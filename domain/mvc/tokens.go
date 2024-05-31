package mvc

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

// TokensUsecase defines an interface for the tokens usecase.
type TokensUsecase interface {
	// GetMetadataByChainDenom returns token metadata for a given chain denom.
	GetMetadataByChainDenom(denom string) (domain.Token, error)

	// GetFullTokenMetadata returns token metadata for all chain denoms as a map.
	GetFullTokenMetadata() (map[string]domain.Token, error)

	// GetChainDenom returns chain denom by human denom
	GetChainDenom(humanDenom string) (string, error)

	// GetChainScalingFactorByDenomMut returns a chain scaling factor for a given denom
	// and a boolean flag indicating whether the scaling factor was found or not.
	// Note that the returned decimal is a shared resource and must not be mutated.
	// A clone should be made for any mutative operation.
	GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error)

	// GetSpotPriceScalingFactorByDenomMut returns the scaling factor for spot price.
	GetSpotPriceScalingFactorByDenom(baseDenom, quoteDenom string) (osmomath.Dec, error)

	// GetPrices returns prices for all given base and quote denoms given a pricing source type or, otherwise, error, if any.
	// The options configure some customization with regards to how prices are computed.
	// By default, the prices are computes by using cache and the default min liquidity parameter set via config.
	// The options are capable of overriding the defaults.
	// The outer map consists of base denoms as keys.
	// The inner map consists of quote denoms as keys.
	// The result of the inner map is prices of the outer base and inner quote.
	GetPrices(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (map[string]map[string]any, error)

	// RegisterPricingStrategy registers a pricing strategy for a given pricing source.
	RegisterPricingStrategy(source domain.PricingSourceType, strategy domain.PricingSource)

	IsValidChainDenom(chainDenom string) bool

	// IsValidPricingSource checks if the pricing source is a valid one
	IsValidPricingSource(pricingSource int) bool

	// GetCoingeckoIdByChainDenom gets the Coingecko ID by chain denom
	GetCoingeckoIdByChainDenom(chainDenom string) (string, error)
}
