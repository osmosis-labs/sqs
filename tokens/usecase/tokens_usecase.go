package usecase

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain/json"
	"github.com/prometheus/client_golang/prometheus"
)

type tokensUseCase struct {
	// Currently, we only expect reads to this shared resource and no writes.
	// If needed, change this to sync.Map in the future.
	// Can be considered for merge with humanToChainDenomMap in the future.
	tokenMetadataByChainDenom map[string]domain.Token
	humanToChainDenomMap      map[string]string
	chainDenoms               map[string]struct{}

	// No mutex since we only expect reads to this shared resource and no writes.
	precisionScalingFactorMap map[int]osmomath.Dec

	// We persist pricing strategies across endpoint calls as they
	// may cache responses internally.
	pricingStrategyMap map[domain.PricingSourceType]domain.PricingSource
}

// Struct to represent the JSON structure
type AssetList struct {
	ChainName string `json:"chainName"`
	Assets    []struct {
		ChainName        string `json:"chainName"`
		SourceDenom      string `json:"sourceDenom"`
		CoinMinimalDenom string `json:"coinMinimalDenom"`
		Symbol           string `json:"symbol"`
		Decimals         int    `json:"decimals"`
		LogoURIs         struct {
			PNG string `json:"png"`
			SVG string `json:"svg"`
		} `json:"logoURIs"`
		CoingeckoID string `json:"coingeckoId"`
		Price       struct {
			PoolID string `json:"poolId"`
			Denom  string `json:"denom"`
		} `json:"price"`
		Categories   []string `json:"categories"`
		PegMechanism string   `json:"pegMechanism"`
		// Found inconsistency in the assest list file
		// TransferMethods/CounterParty sometimes is an array and sometimes is an object
		// So commenting out the fields for now
		// TransferMethods []struct {
		// 	Name         string `json:"name"`
		// 	Type         string `json:"type"`
		// 	DepositURL   string `json:"depositUrl"`
		// 	WithdrawURL  string `json:"withdrawUrl"`
		// 	CounterParty []struct {
		// 		ChainName        string `json:"chainName"`
		// 		ChainID          string `json:"chainId"`
		// 		SourceDenom      string `json:"sourceDenom"`
		// 		Port             string `json:"port"`
		// 		ChannelID        string `json:"channelId"`
		// 		WrappedAssetID   string `json:"wrappedAssetId"`
		// 		UnwrappedAssetID string `json:"unwrappedAssetId"`
		// 		EvmChainID       int    `json:"evmChainId"`
		// 		SourceChainID    string `json:"sourceChainId"`
		// 	} `json:"counterparty"`
		// 	Chain []struct {
		// 		Port      string `json:"port"`
		// 		ChannelID string `json:"channelId"`
		// 		Path      string `json:"path"`
		// 	} `json:"chain"`
		// 	WrappedAssetID   string `json:"wrappedAssetId"`
		// 	UnwrappedAssetID string `json:"unwrappedAssetId"`
		// } `json:"transferMethods"`
		// CounterParty []struct {
		// 	ChainName   string `json:"chainName"`
		// 	SourceDenom string `json:"sourceDenom"`
		// 	ChainType   string `json:"chainType"`
		// 	ChainID     string `json:"chainId"`
		// 	Address     string `json:"address"`
		// 	Symbol      string `json:"symbol"`
		// 	Decimals    int    `json:"decimals"`
		// 	LogoURIs    struct {
		// 		PNG string `json:"png"`
		// 		SVG string `json:"svg"`
		// 	} `json:"logoURIs"`
		// } `json:"counterparty"`
		VariantGroupKey string `json:"variantGroupKey"`
		Name            string `json:"name"`
		Verified        bool   `json:"verified"`
		Unstable        bool   `json:"unstable"`
		Disabled        bool   `json:"disabled"`
		Preview         bool   `json:"preview"`
		SortWith        struct {
			ChainName   string `json:"chainName"`
			SourceDenom string `json:"sourceDenom"`
		} `json:"sortWith"`
	} `json:"assets"`
}

// Define a result struct to hold the quoteDenom and the fetched price or error
type priceResult struct {
	quoteDenom string
	price      osmomath.BigDec
	err        error
}

// Define a result struct to hold the base denom and prices for each possible quote denom or error
type priceResults struct {
	baseDenom string
	prices    map[string]any
	err       error
}

var _ mvc.TokensUsecase = &tokensUseCase{}

const (
	unlistedKeyword = "osmosis-unlisted"
)

var (
	tenDec = osmomath.NewDec(10)

	pricingErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_errors_total",
			Help: "Total number of pricing errors",
		},
		[]string{"base", "quote", "err"},
	)
)

// NewTokensUsecase will create a new tokens use case object
func NewTokensUsecase(tokenMetadataByChainDenom map[string]domain.Token) mvc.TokensUsecase {
	// Create human denom to chain denom map
	humanToChainDenomMap := make(map[string]string, len(tokenMetadataByChainDenom))
	uniquePrecisionMap := make(map[int]struct{}, 0)
	chainDenoms := map[string]struct{}{}

	for chainDenom, tokenMetadata := range tokenMetadataByChainDenom {
		// lower case human denom
		lowerCaseHumanDenom := strings.ToLower(tokenMetadata.HumanDenom)

		humanToChainDenomMap[lowerCaseHumanDenom] = chainDenom

		uniquePrecisionMap[tokenMetadata.Precision] = struct{}{}

		chainDenoms[chainDenom] = struct{}{}
	}

	// Precompute precision scaling factors
	precisionScalingFactors := make(map[int]osmomath.Dec, len(uniquePrecisionMap))
	for precision := range uniquePrecisionMap {
		precisionScalingFactors[precision] = tenDec.Power(uint64(precision))
	}

	return &tokensUseCase{
		tokenMetadataByChainDenom: tokenMetadataByChainDenom,
		humanToChainDenomMap:      humanToChainDenomMap,
		precisionScalingFactorMap: precisionScalingFactors,

		pricingStrategyMap: map[domain.PricingSourceType]domain.PricingSource{},

		chainDenoms: chainDenoms,
	}
}

// GetChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) GetChainDenom(humanDenom string) (string, error) {
	humanDenomLowerCase := strings.ToLower(humanDenom)

	chainDenom, ok := t.humanToChainDenomMap[humanDenomLowerCase]
	if !ok {
		return "", fmt.Errorf("chain denom for human denom (%s) is not found", humanDenomLowerCase)
	}

	return chainDenom, nil
}

// GetMetadataByChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) GetMetadataByChainDenom(denom string) (domain.Token, error) {
	token, ok := t.tokenMetadataByChainDenom[denom]
	if !ok {
		return domain.Token{}, fmt.Errorf("metadata for denom (%s) is not found", denom)
	}

	return token, nil
}

// GetFullTokenMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) GetFullTokenMetadata() (map[string]domain.Token, error) {
	// Do a copy of the cached metadata
	result := make(map[string]domain.Token, len(t.tokenMetadataByChainDenom))
	for denom, tokenMetadata := range t.tokenMetadataByChainDenom {
		result[denom] = tokenMetadata
	}

	return result, nil
}

// GetChainScalingFactorByDenomMut implements mvc.TokensUsecase.
func (t *tokensUseCase) GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error) {
	denomMetadata, err := t.GetMetadataByChainDenom(denom)
	if err != nil {
		return osmomath.Dec{}, err
	}

	scalingFactor, ok := t.getChainScalingFactorMut(denomMetadata.Precision)
	if !ok {
		return osmomath.Dec{}, fmt.Errorf("scalng factor for precision (%d) and denom (%s) not found", denomMetadata.Precision, denom)
	}

	return scalingFactor, nil
}

// GetPrices implements pricing.PricingStrategy.
func (t *tokensUseCase) GetPrices(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (map[string]map[string]any, error) {
	byBaseDenomResult := make(map[string]map[string]any, len(baseDenoms))

	// Create a channel to communicate the results
	resultsChan := make(chan priceResults, len(quoteDenoms))

	// Use a WaitGroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	// For every base denom, create a map with quote denom prices.
	for _, baseDenom := range baseDenoms {
		wg.Add(1)
		go func(baseDenom string) {
			defer wg.Done()

			prices, err := t.getPricesForBaseDenom(ctx, baseDenom, quoteDenoms, pricingSourceType, opts...)
			if err != nil {
				// This should not panic, so just logging the error here and continue
				fmt.Println(err.Error())
			}
			resultsChan <- priceResults{baseDenom: baseDenom, prices: prices, err: err}
		}(baseDenom)
	}

	// Close the results channel once all goroutines have finished
	go func() {
		wg.Wait()          // Wait for all goroutines to finish
		close(resultsChan) // Close the channel
	}()

	// Read from the results channel and update the map
	for range baseDenoms {
		result := <-resultsChan

		if result.err != nil {
			return nil, result.err
		}
		byBaseDenomResult[result.baseDenom] = result.prices
	}

	return byBaseDenomResult, nil
}

// getPricesForBaseDenom fetches all prices for base denom given a slice of quotes and pricing options.
// Pricing options determine whether to recompute prices or use the cache as well as the desired source of prices.
// Returns a map with keys as quotes and values as prices or error, if any.
// Returns error if base denom is not found in the token metadata.
// Sets the price to zero in case of failing to compute the price between base and quote but these being valid tokens.
func (t *tokensUseCase) getPricesForBaseDenom(ctx context.Context, baseDenom string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, pricingOptions ...domain.PricingOption) (map[string]any, error) {
	byQuoteDenomForGivenBaseResult := make(map[string]any, len(quoteDenoms))
	// Validate base denom is a valid denom
	// Return zeroes for all quotes if base denom is not found
	_, err := t.GetMetadataByChainDenom(baseDenom)
	if err != nil {
		for _, quoteDenom := range quoteDenoms {
			byQuoteDenomForGivenBaseResult[quoteDenom] = osmomath.ZeroBigDec()
		}
		return byQuoteDenomForGivenBaseResult, nil
	}

	// Create a channel to communicate the results
	resultsChan := make(chan priceResult, len(quoteDenoms))

	// Get the pricing strategy
	pricingStrategy, ok := t.pricingStrategyMap[pricingSourceType]
	if !ok {
		return nil, fmt.Errorf("pricing strategy (%s) not found in the tokens use case", pricingStrategy)
	}

	// Use a WaitGroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Given the current base denom, compute all of its prices with the quotes
	for _, quoteDenom := range quoteDenoms {
		wg.Add(1)
		go func(baseDenom, quoteDenom string) {
			defer wg.Done()

			price, err := pricingStrategy.GetPrice(ctx, baseDenom, quoteDenom, pricingOptions...)
			resultsChan <- priceResult{quoteDenom, price, err}
		}(baseDenom, quoteDenom)
	}

	// Close the results channel once all goroutines have finished
	go func() {
		wg.Wait()          // Wait for all goroutines to finish
		close(resultsChan) // Close the channel
	}()

	// Read from the results channel and update the map
	for range quoteDenoms {
		result := <-resultsChan

		if result.err != nil {
			// Increase prometheus counter
			pricingErrorCounter.WithLabelValues(baseDenom, result.quoteDenom, result.err.Error()).Inc()

			// Set the price to zero in case of error
			result.price = osmomath.ZeroBigDec()
		}
		byQuoteDenomForGivenBaseResult[result.quoteDenom] = result.price
	}

	return byQuoteDenomForGivenBaseResult, nil
}

func (t *tokensUseCase) getChainScalingFactorMut(precision int) (osmomath.Dec, bool) {
	result, ok := t.precisionScalingFactorMap[precision]
	return result, ok
}

// GetTokensFromChainRegistry fetches the tokens from the chain registry.
// It returns a map of tokens by chain denom.
func GetTokensFromChainRegistry(chainRegistryAssetsFileURL string) (map[string]domain.Token, error) {
	// Fetch the JSON data from the URL
	response, err := http.Get(chainRegistryAssetsFileURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	// Decode the JSON data
	var assetList AssetList
	err = json.NewDecoder(response.Body).Decode(&assetList)
	if err != nil {
		return nil, err
	}

	tokensByChainDenom := make(map[string]domain.Token)

	// Iterate through each asset and its denom units to print exponents
	for _, asset := range assetList.Assets {
		token := domain.Token{}
		token.Precision = asset.Decimals
		token.HumanDenom = asset.Symbol
		token.IsUnlisted = asset.Disabled
		tokensByChainDenom[asset.CoinMinimalDenom] = token
	}

	return tokensByChainDenom, nil
}

// GetSpotPriceScalingFactorByDenomMut implements mvc.TokensUsecase.
func (t *tokensUseCase) GetSpotPriceScalingFactorByDenom(baseDenom string, quoteDenom string) (osmomath.Dec, error) {
	baseScalingFactor, err := t.GetChainScalingFactorByDenomMut(baseDenom)
	if err != nil {
		return osmomath.Dec{}, err
	}

	quoteScalingFactor, err := t.GetChainScalingFactorByDenomMut(quoteDenom)
	if err != nil {
		return osmomath.Dec{}, err
	}

	if quoteScalingFactor.IsZero() {
		return osmomath.Dec{}, fmt.Errorf("scaling factor for quote denom (%s) is zero", quoteDenom)
	}

	return baseScalingFactor.Quo(quoteScalingFactor), nil
}

// RegisterPricingStrategy implements mvc.TokensUsecase.
func (t *tokensUseCase) RegisterPricingStrategy(source domain.PricingSourceType, strategy domain.PricingSource) {
	t.pricingStrategyMap[source] = strategy
}

// IsValidChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) IsValidChainDenom(chainDenom string) bool {
	metaData, ok := t.tokenMetadataByChainDenom[chainDenom]
	return ok && !metaData.IsUnlisted
}
