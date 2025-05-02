package usecase

import "github.com/osmosis-labs/sqs/domain"

// PutArbitraryTypeTokenMetadata is a test helper to put arbitrary types to token metadata
func (t *TokensUseCase) SetTokenMetadataByChainDenom(key string, value any) {
	t.tokenMetadataByChainDenom.Store(key, value)
}

// PutArbitraryTypeHumanToChainDenomMap is a test helper to put arbitrary types to human to chain denom map
func (t *TokensUseCase) SetTypeHumanToChainDenomMap(key string, value any) {
	t.humanToChainDenomMap.Store(key, value)
}

// SetChainDenoms is a test helper to put arbitrary types to chain denoms
func (t *TokensUseCase) SetChainDenoms(key any, value any) {
	t.chainDenoms.Store(key, value)
}

// SetCoingeckoIDs is a test helper to put arbitrary types to coingecko ids
func (t *TokensUseCase) SetCoingeckoIDs(key string, value any) {
	t.coingeckoIds.Store(key, value)
}

// SetTokensPriceFetcher is a test helper to set tokens price fetcher.
func (t *TokensUseCase) SetTokensPriceFetcher(fetcher domain.TokensPriceFetcher) {
	t.tokenPriceFetcher = fetcher
}

// SetLastFetchHash is a test helper to set last fetch hash.
func (f *ChainRegistryHTTPFetcher) SetLastFetchHash(value string) {
	f.lastFetchHash = value
}

// GetLastFetchHash is a test helper to get last fetch hash
func (f *ChainRegistryHTTPFetcher) GetLastFetchHash() string {
	return f.lastFetchHash
}
