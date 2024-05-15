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
	// TotalLiquidityUSDC represents the total liquidity in USDC across all pools.
	// If it is set to zero, that there was a failure in fetching the price.
	// @Type string
	TotalLiquidityUSDC osmomath.Int `json:"total_liquidity_usdc"`
}
