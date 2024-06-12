package domain

// PoolDenomMetaDataMap defines the map of pool denom metadata.
// [chain denom] => pool denom metadata
// Note: BREAKING API - this is an API breaking type as it is serialized as an output
// of tokens/pool-metadata. Be mindful of changing it without
// separating the API response for backward compatibility.
type PoolDenomMetaDataMap map[string]PoolDenomMetaData

// Set sets the total liquidity and total liquidity in USDC for the given denom.
func (p PoolDenomMetaDataMap) Set(denom string, poolDenomMetadata PoolDenomMetaData) {
	p[denom] = poolDenomMetadata
}
