package usecase

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/osmosis-labs/sqs/domain"
)

// GetTokensFromChainRegistryFunc is a GetTokensFromChainRegistry function signature.
type GetTokensFromChainRegistryFunc func(chainRegistryAssetsFileURL string) (map[string]domain.Token, string, error)

// GetTokensFromChainRegistry fetches the tokens from the chain registry.
// It returns a map of tokens by chain denom.
func GetTokensFromChainRegistry(chainRegistryAssetsFileURL string) (map[string]domain.Token, string, error) {
	// Fetch the JSON data from the URL
	response, err := http.Get(chainRegistryAssetsFileURL)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()

	// Decode the JSON data
	var assetList AssetList
	err = json.NewDecoder(response.Body).Decode(&assetList)
	if err != nil {
		return nil, "", err
	}

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, "", err
	}
	response.Body.Close() //  close old response body
	response.Body = io.NopCloser(bytes.NewBuffer(data))

	tokensByChainDenom := make(map[string]domain.Token)

	// Iterate through each asset and its denom units to print exponents
	for _, asset := range assetList.Assets {
		token := domain.Token{}
		token.Precision = asset.Decimals
		token.HumanDenom = asset.Symbol
		token.IsUnlisted = asset.Preview
		token.CoingeckoID = asset.CoingeckoID
		tokensByChainDenom[asset.CoinMinimalDenom] = token
	}

	return tokensByChainDenom, fmt.Sprintf("%x", md5.Sum(data)), nil
}

// TokenRegistryLoader is loader of tokens from the chain registry passing results to the loadTokens function.
type TokenRegistryLoader interface {
	// FetchAndUpdateTokens fetches tokens from the chain registry and loads by calling loadTokens if there are changes.
	FetchAndUpdateTokens(loadTokens LoadTokensFunc) error
}

// ChainRegistryHTTPFetcher is an implementation of TokenRegistryLoader that fetches tokens from the HTTP chain registry.
type ChainRegistryHTTPFetcher struct {
	registryURL                string
	getTokensFromChainRegistry GetTokensFromChainRegistryFunc
	lastFetchHash              string
}

// NewChainRegistryHTTPFetcher creates a new instance of ChainRegistryHTTPFetcher.
func NewChainRegistryHTTPFetcher(registryURL string, getTokensFromChainRegistry GetTokensFromChainRegistryFunc) *ChainRegistryHTTPFetcher {
	return &ChainRegistryHTTPFetcher{
		getTokensFromChainRegistry: getTokensFromChainRegistry,
		registryURL:                registryURL,
	}
}

// FetchAndUpdateTokensFunc is a FetchAndUpdateTokens function signature.
type FetchAndUpdateTokensFunc func(loadTokens func(tokens map[string]domain.Token)) error

// FetchAndUpdateTokens fetches tokens from the chain registry and loads by calling loadTokens  function.
// In case there were no changes since last fetch, it does not call loadTokens.
func (f *ChainRegistryHTTPFetcher) FetchAndUpdateTokens(loadTokens LoadTokensFunc) error {
	tokens, hash, err := f.getTokensFromChainRegistry(f.registryURL)
	if err != nil {
		return err
	}

	if f.lastFetchHash != hash {
		loadTokens(tokens)
		f.lastFetchHash = hash
	}

	return nil
}
