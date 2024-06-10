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

// DenomPoolLiquidityMap is a map of denoms to their pool liquidity data.
type DenomPoolLiquidityMap map[string]DenomPoolLiquidityData

// DenomPoolLiquidityData contains the pool liquidity data for a denom
// It has the total liquidity for the denom as well as all the
// pools with their individual contributions to the total.
type DenomPoolLiquidityData struct {
	// Total liquidity for this denom
	TotalLiquidity osmomath.Int
	// Mapping from pool ID to denom liquidity
	// in that pool
	Pools map[uint64]osmomath.Int
}
