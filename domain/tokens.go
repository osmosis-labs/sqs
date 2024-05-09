package domain

// Token represents the token's domain model
type Token struct {
	// HumanDenom is the human readable denom.
	HumanDenom string `json:"human_denom"`
	// Precision is the precision of the token.
	Precision int `json:"precision"`
	// IsUnlisted is true if the token is unlisted.
	IsUnlisted  bool   `json:"is_unlisted"`
	CoingeckoID string `json:"coingecko_id"`
}
