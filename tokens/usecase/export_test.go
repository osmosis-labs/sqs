package usecase

// PutArbitraryTypeTokenMetadata is a test helper to put arbitrary types to token metadata
func (t *tokensUseCase) SetTokenMetadataByChainDenom(key string, value any) {
	t.tokenMetadataByChainDenom.Store(key, value)
}

// PutArbitraryTypeHumanToChainDenomMap is a test helper to put arbitrary types to human to chain denom map
func (t *tokensUseCase) SetTypeHumanToChainDenomMap(key string, value any) {
	t.humanToChainDenomMap.Store(key, value)
}

// SetChainDenoms is a test helper to put arbitrary types to chain denoms
func (t *tokensUseCase) SetChainDenoms(key any, value any) {
	t.chainDenoms.Store(key, value)
}

// SetCoingeckoIDs is a test helper to put arbitrary types to coingecko ids
func (t *tokensUseCase) SetCoingeckoIDs(key string, value any) {
	t.coingeckoIds.Store(key, value)
}

// SetLastFetchHash is a test helper to set last fetch hash
func (f *ChainRegistryHTTPFetcher) SetLastFetchHash(value string) {
	f.lastFetchHash = value
}

// GetLastFetchHash is a test helper to get last fetch hash
func (f *ChainRegistryHTTPFetcher) GetLastFetchHash() string {
	return f.lastFetchHash
}
