package domain

import "github.com/osmosis-labs/osmosis/osmomath"

// Token represents the token's domain model
type Token struct {
	// HumanDenom is the human readable denom.
	HumanDenom string `json:"human_denom"`
	// Precision is the precision of the token.
	Precision int `json:"precision"`
	// IsUnlisted is true if the token is unlisted.
	IsUnlisted bool `json:"is_unlisted"`
}

// PoolDenomMetaData contains the metadata about the denoms collected from the pools.
type PoolDenomMetaData struct {
	// TotalLiquidity represents the total liquidity across all pools.
	// @Type string
	TotalLiquidity osmomath.Int `json:"total_liquidity"`
	// TotalLiquidityCap represents the total liquidity capitalization across all pools.
	// If it is set to zero, that there was a failure in fetching the price.
	// @Type string
	TotalLiquidityCap osmomath.Int `json:"total_liquidity_cap"`
}

type DenomLiquidityMap map[string]DenomLiquidityData

// DenomLiquidityData contains the liquidity data for a denom
type DenomLiquidityData struct {
	// Total liquidity for this denom
	TotalLiquidity osmomath.Int
	// Mapping from pool ID to denom liquidity
	// in that pool
	Pools map[uint64]osmomath.Int
}
