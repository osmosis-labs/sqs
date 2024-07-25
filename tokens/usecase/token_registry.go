package usecase

import (
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

	// read the response body once to be used for
	// decoding and for checksum
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, "", err
	}

	// Calculate the MD5 checksum of the data
	checksum := fmt.Sprintf("%x", md5.Sum(data))

	// Decode the JSON data
	var assetList AssetList
	err = json.Unmarshal(data, &assetList)
	if err != nil {
		return nil, "", err
	}

	tokensByChainDenom := make(map[string]domain.Token)

	// Iterate through each asset and its denom units to print exponents
	for _, asset := range assetList.Assets {
		token := domain.Token{}
		token.Precision = asset.Decimals
		token.HumanDenom = asset.Symbol
		token.IsUnlisted = asset.Preview
		token.CoingeckoID = asset.CoingeckoID
		token.Name = asset.Name
		token.Denom = asset.CoinMinimalDenom
		tokensByChainDenom[asset.CoinMinimalDenom] = token
	}

	return tokensByChainDenom, checksum, nil
}

// ChainRegistryHTTPFetcher is an implementation of TokenRegistryLoader that fetches tokens from the HTTP chain registry.
type ChainRegistryHTTPFetcher struct {
	registryURL                string
	getTokensFromChainRegistry GetTokensFromChainRegistryFunc
	loadTokens                 LoadTokensFunc
	lastFetchHash              string
}

// NewChainRegistryHTTPFetcher creates a new instance of ChainRegistryHTTPFetcher.
func NewChainRegistryHTTPFetcher(registryURL string, getTokensFromChainRegistry GetTokensFromChainRegistryFunc, loadTokens LoadTokensFunc) *ChainRegistryHTTPFetcher {
	return &ChainRegistryHTTPFetcher{
		registryURL:                registryURL,
		getTokensFromChainRegistry: getTokensFromChainRegistry,
		loadTokens:                 loadTokens,
	}
}

// FetchAndUpdateTokens fetches tokens from the chain registry and updates the token registry.
// In case there were no changes since last fetch, it does update the token registry.
func (f *ChainRegistryHTTPFetcher) FetchAndUpdateTokens() error {
	tokens, hash, err := f.getTokensFromChainRegistry(f.registryURL)
	if err != nil {
		return err
	}

	if f.lastFetchHash != hash {
		f.loadTokens(tokens)
		f.lastFetchHash = hash
	}

	return nil
}
