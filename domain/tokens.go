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
	// LocalMCap represents the local market cap.
	// @Type string
	LocalMCap osmomath.Int `json:"local_mcap"`
}
