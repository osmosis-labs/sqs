package mvc

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

type TokensPoolLiquidityHandler interface {
	// GetChainScalingFactorByDenomMut returns a chain scaling factor for a given denom
	// and a boolean flag indicating whether the scaling factor was found or not.
	// Note that the returned decimal is a shared resource and must not be mutated.
	// A clone should be made for any mutative operation.
	GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error)

	// UpdatePoolDenomMetadata updates the pool denom metadata, completely overwriting any previous
	// denom results stored internally, if any. The denoms metadata that is present internally
	// but not in the provided map will be left unchanged.
	UpdatePoolDenomMetadata(tokensMetadata domain.PoolDenomMetaDataMap)
}

// TokensUsecase defines an interface for the tokens usecase.
type TokensUsecase interface {
	TokensPoolLiquidityHandler

	// GetMetadataByChainDenom returns token metadata for a given chain denom.
	GetMetadataByChainDenom(denom string) (domain.Token, error)

	// GetFullTokenMetadata returns token metadata for all chain denoms as a map.
	GetFullTokenMetadata() (map[string]domain.Token, error)

	// GetChainDenom returns chain denom by human denom
	GetChainDenom(humanDenom string) (string, error)

	// GetSpotPriceScalingFactorByDenomMut returns the scaling factor for spot price.
	GetSpotPriceScalingFactorByDenom(baseDenom, quoteDenom string) (osmomath.Dec, error)

	// GetPrices returns prices for all given base and quote denoms given a pricing source type or, otherwise, error, if any.
	// The options configure some customization with regards to how prices are computed.
	// By default, the prices are computes by using cache and the default min liquidity parameter set via config.
	// The options are capable of overriding the defaults.
	// The outer map consists of base denoms as keys.
	// The inner map consists of quote denoms as keys.
	// The result of the inner map is prices of the outer base and inner quote.
	GetPrices(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error)

	// GetMinPoolLiquidityCap returns the min pool liquidity capitalization between the two denoms.
	// Returns error if there is no pool liquidity metadata for one of the tokens.
	// Returns error if pool liquidity metadata is large enough to cause overflow.
	GetMinPoolLiquidityCap(denomA, denomB string) (uint64, error)

	// GetPoolDenomMetadata returns the pool denom metadata of a pool denom.
	// This metadata is accumulated from all pools.
	GetPoolDenomMetadata(chainDenom string) (domain.PoolDenomMetaData, error)

	// GetPoolLiquidityCap returns the pool liquidity market cap for a given chain denom.
	// This value is accumulated from all Osmosis pools.
	GetPoolLiquidityCap(chainDenom string) (osmomath.Int, error)

	// GetPoolDenomsMetadata returns the pool denom metadata for the given chain denoms.
	// These values are accumulated from all Osmosis pools.
	GetPoolDenomsMetadata(chainDenoms []string) domain.PoolDenomMetaDataMap

	// GetFullPoolDenomMetadata returns the local market caps for all chain denoms.
	// For any valid (per the asset list) denom, if there is no metadata, it will be set to empty
	// and all values such as local market cap will be set to zero.
	GetFullPoolDenomMetadata() domain.PoolDenomMetaDataMap

	// RegisterPricingStrategy registers a pricing strategy for a given pricing source.
	RegisterPricingStrategy(source domain.PricingSourceType, strategy domain.PricingSource)

	IsValidChainDenom(chainDenom string) bool

	// IsValidPricingSource checks if the pricing source is a valid one
	IsValidPricingSource(pricingSource int) bool

	// GetCoingeckoIdByChainDenom gets the Coingecko ID by chain denom
	GetCoingeckoIdByChainDenom(chainDenom string) (string, error)
}

// ValidateChainDenomQueryParam validates the chain denom query parameter.
// If isHumanDenoms is true, it converts the human denom to chain denom.
// If isHumanDenoms is false, it validates the chain denom.
// Returns the chain denom and an error if any.
func ValidateChainDenomQueryParam(tokensUsecase TokensUsecase, denom string, isHumanDenoms bool) (string, error) {
	// Note that sdk.Coins initialization
	// auto-converts base denom from human
	// to IBC notation.
	// As a result, we avoid attempting the
	// to convert a denom that is already changed.
	baseDenom, err := sdk.GetBaseDenom()
	if err != nil {
		return "", nil
	}

	if isHumanDenoms {
		// Convert human denom to chain denom.
		// See definition of baseDenom.
		if denom != baseDenom {
			return tokensUsecase.GetChainDenom(denom)
		}
	} else {
		if !tokensUsecase.IsValidChainDenom(denom) {
			return "", fmt.Errorf("denom is not a valid chain denom (%s)", denom)
		}
	}

	// Valid chain denom
	return denom, nil
}

// ValidateChainDenomsQueryParam validates the chain denom query parameters.
func ValidateChainDenomsQueryParam(c echo.Context, tokensUsecase TokensUsecase, denoms []string) ([]string, error) {
	isHumanDenoms, err := domain.GetIsHumanDenomsQueryParam(c)
	if err != nil {
		return nil, err
	}

	chainDenoms := make([]string, len(denoms))
	for i, denom := range denoms {
		chainDenom, err := ValidateChainDenomQueryParam(tokensUsecase, denom, isHumanDenoms)
		if err != nil {
			return nil, err
		}
		chainDenoms[i] = chainDenom
	}
	return chainDenoms, nil
}
