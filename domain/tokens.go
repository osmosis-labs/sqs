package domain

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
