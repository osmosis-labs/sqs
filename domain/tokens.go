package domain

import "github.com/osmosis-labs/osmosis/osmomath"

// Token represents the token's domain model
type Token struct {
	// HumanDenom is the human readable denom.
	HumanDenom string `json:"symbol"`
	// Precision is the precision of the token.
	Precision int `json:"decimals"`
	// IsUnlisted is true if the token is unlisted.
	IsUnlisted  bool   `json:"preview"`
	CoingeckoID string `json:"coingeckoId"`
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
	// Price represents the price of the token.
	// @Type string
	Price osmomath.BigDec `json:"price"`
}

// DenomLiquidityMap is a map from denom to liquidity data
type DenomLiquidityMap map[string]DenomLiquidityData

// DenomLiquidityData contains the liquidity data for a denom
// It has the total liquidity for the denom as well as all the
// pools with their individual contributions to the total.
type DenomLiquidityData struct {
	// Total liquidity for this denom
	TotalLiquidity osmomath.Int
	// Mapping from pool ID to denom liquidity
	// in that pool
	Pools map[uint64]osmomath.Int
}
