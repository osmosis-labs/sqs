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
	"github.com/osmosis-labs/sqs/domain/workerpool"
	"github.com/osmosis-labs/sqs/sqsdomain/json"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Max number of workers to fetch prices concurrently
	// TODO: move to config
	maxNumWorkes = 10
)

type tokensUseCase struct {
	// Can be considered for merge with humanToChainDenomMap in the future.
	tokenMetadataByChainDenom sync.Map // domain.Token
	humanToChainDenomMap      sync.Map // string
	chainDenoms               sync.Map // struct{}

	// No mutex since we only expect reads to this shared resource and no writes.
	precisionScalingFactorMap sync.Map // map[int]osmomath.Dec

	// Metadata about denoms that is collected from the pools.
	// E.g. total denom liquidity across all pools.
	poolDenomMetaData sync.Map

	// We persist pricing strategies across endpoint calls as they
	// may cache responses internally.
	pricingStrategyMap map[domain.PricingSourceType]domain.PricingSource

	// Map of chain denoms to coingecko IDs
	coingeckoIds sync.Map // map[string]string

	// Represents the interval at which to update the assets from the chain registry
	updateAssetsHeightInterval int

	// Represents the URL to fetch the chain registry assets file
	chainRegistryAssetsFileURL string
}

// Struct to represent the JSON structure
type AssetList struct {
	ChainName string `json:"chainName"`
	Assets    []struct {
		CoinMinimalDenom string `json:"coinMinimalDenom"`
		Symbol           string `json:"symbol"`
		Decimals         int    `json:"decimals"`
		CoingeckoID      string `json:"coingeckoId"`
		Preview          bool   `json:"preview"`
	} `json:"assets"`
}

// Define a result struct to hold the base denom and prices for each possible quote denom or error
type priceResults struct {
	baseDenom string
	prices    map[string]osmomath.BigDec
	err       error
}

var _ mvc.TokensUsecase = &tokensUseCase{}

var (
	tenDec = osmomath.NewDec(10)

	pricingErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_errors_total",
			Help: "Total number of pricing errors",
		},
		[]string{"base", "quote", "err"},
	)
	fallbackCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_fallback_total",
			Help: "Total number of fallback from chain pricing source to coingecko",
		},
		[]string{"base", "quote"},
	)
)

// NewTokensUsecase will create a new tokens use case object
func NewTokensUsecase(tokenMetadataByChainDenom map[string]domain.Token, updateAssetsHeightInterval int, chainRegistryAssetsFileURL string) mvc.TokensUsecase {
	us := tokensUseCase{
		pricingStrategyMap:         map[domain.PricingSourceType]domain.PricingSource{},
		poolDenomMetaData:          sync.Map{},
		updateAssetsHeightInterval: updateAssetsHeightInterval,
		chainRegistryAssetsFileURL: chainRegistryAssetsFileURL,
	}

	us.LoadTokens(tokenMetadataByChainDenom)

	return &us
}

// LoadTokens implements mvc.TokensUsecase.
func (t *tokensUseCase) LoadTokens(tokenMetadataByChainDenom map[string]domain.Token) {
	// Create human denom to chain denom map
	uniquePrecisionMap := make(map[int]struct{}, 0)

	for chainDenom, tokenMetadata := range tokenMetadataByChainDenom {
		// lower case human denom
		lowerCaseHumanDenom := strings.ToLower(tokenMetadata.HumanDenom)

		t.humanToChainDenomMap.Store(lowerCaseHumanDenom, chainDenom)
		t.tokenMetadataByChainDenom.Store(chainDenom, tokenMetadata)

		uniquePrecisionMap[tokenMetadata.Precision] = struct{}{}

		t.chainDenoms.Store(chainDenom, struct{}{})

		t.coingeckoIds.Store(chainDenom, tokenMetadata.CoingeckoID)
	}

	// Precompute precision scaling factors
	for precision := range uniquePrecisionMap {
		t.precisionScalingFactorMap.Store(precision, tenDec.Power(uint64(precision)))
	}
}

// UpdatePoolDenomMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) UpdatePoolDenomMetadata(poolDenomMetadata domain.PoolDenomMetaDataMap) {
	for chainDenom, tokenMetadata := range poolDenomMetadata {
		t.poolDenomMetaData.Store(chainDenom, tokenMetadata)
	}
}

// GetPoolLiquidityCap implements mvc.TokensUsecase.
func (t *tokensUseCase) GetPoolLiquidityCap(chainDenom string) (osmomath.Int, error) {
	poolDenomMetadata, err := t.GetPoolDenomMetadata(chainDenom)
	if err != nil {
		return osmomath.Int{}, err
	}
	return poolDenomMetadata.TotalLiquidity, nil
}

// GetPoolDenomMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) GetPoolDenomMetadata(chainDenom string) (domain.PoolDenomMetaData, error) {
	poolDenomMetadataObj, ok := t.poolDenomMetaData.Load(chainDenom)
	if !ok {
		return domain.PoolDenomMetaData{}, domain.PoolDenomMetaDataNotPresentError{
			ChainDenom: chainDenom,
		}
	}

	poolDenomMetadata, ok := poolDenomMetadataObj.(domain.PoolDenomMetaData)
	if !ok {
		return domain.PoolDenomMetaData{}, fmt.Errorf("pool denom metadata for denom (%s) is not of type domain.PoolDenomMetaData", chainDenom)
	}

	return poolDenomMetadata, nil
}

// GetPoolDenomsMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) GetPoolDenomsMetadata(chainDenoms []string) domain.PoolDenomMetaDataMap {
	result := make(domain.PoolDenomMetaDataMap, len(chainDenoms))

	for _, chainDenom := range chainDenoms {
		poolDenomMetadata, err := t.GetPoolDenomMetadata(chainDenom)

		// Instead of failing the entire request, we just set the results to zero
		if err != nil {
			result.Set(chainDenom, domain.PoolDenomMetaData{
				TotalLiquidity:    osmomath.ZeroInt(),
				TotalLiquidityCap: osmomath.ZeroInt(),
				Price:             osmomath.ZeroBigDec(),
			})
		} else {
			// Otherwise, we set the correct value
			result[chainDenom] = poolDenomMetadata
		}
	}

	return result
}

// GetFullPoolDenomMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) GetFullPoolDenomMetadata() domain.PoolDenomMetaDataMap {
	var chainDenoms []string
	t.chainDenoms.Range(func(chainDenom, _ any) bool {
		chainDenoms = append(chainDenoms, chainDenom.(string))
		return true
	})
	return t.GetPoolDenomsMetadata(chainDenoms)
}

// GetChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) GetChainDenom(humanDenom string) (string, error) {
	humanDenomLowerCase := strings.ToLower(humanDenom)

	chainDenom, ok := t.humanToChainDenomMap.Load(humanDenomLowerCase)
	if !ok {
		return "", fmt.Errorf("chain denom for human denom (%s) is not found", humanDenomLowerCase)
	}

	return chainDenom.(string), nil
}

// GetMetadataByChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) GetMetadataByChainDenom(denom string) (domain.Token, error) {
	token, ok := t.tokenMetadataByChainDenom.Load(denom)
	if !ok {
		return domain.Token{}, fmt.Errorf("metadata for denom (%s) is not found", denom)
	}

	return token.(domain.Token), nil
}

// GetFullTokenMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) GetFullTokenMetadata() (map[string]domain.Token, error) {
	// Do a copy of the cached metadata
	result := make(map[string]domain.Token)
	t.tokenMetadataByChainDenom.Range(func(denom, token any) bool {
		result[denom.(string)] = token.(domain.Token)
		return true
	})
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
func (t *tokensUseCase) GetPrices(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
	byBaseDenomResult := make(map[string]map[string]osmomath.BigDec, len(baseDenoms))

	numWorkers := len(baseDenoms)
	if numWorkers > maxNumWorkes {
		numWorkers = maxNumWorkes
	}

	basePriceDispatcher := workerpool.NewDispatcher[priceResults](numWorkers)
	go basePriceDispatcher.Run()

	// For every base denom, create a map with quote denom prices.
	for _, baseDenom := range baseDenoms {
		baseDenom := baseDenom

		basePriceDispatcher.JobQueue <- workerpool.Job[priceResults]{
			Task: func() (priceResults, error) {
				var err error
				defer func() {
					// Recover from panic if one occurred
					if r := recover(); r != nil {
						err = fmt.Errorf("panic in GetPrices: %v", r)
					}
				}()

				prices, err := t.getPricesForBaseDenom(ctx, baseDenom, quoteDenoms, pricingSourceType, opts...)
				if err != nil {
					// This should not panic, so just logging the error here and continue
					fmt.Println(err.Error())
				}

				return priceResults{
					baseDenom: baseDenom,
					prices:    prices,
					err:       err,
				}, nil
			},
		}
	}

	// Read from the results channel and update the map
	for range baseDenoms {
		result := <-basePriceDispatcher.ResultQueue

		if result.Result.err != nil {
			return nil, result.Result.err
		}
		byBaseDenomResult[result.Result.baseDenom] = result.Result.prices
	}

	return byBaseDenomResult, nil
}

// getPricesForBaseDenom fetches all prices for base denom given a slice of quotes and pricing options.
// Pricing options determine whether to recompute prices or use the cache as well as the desired source of prices.
// Returns a map with keys as quotes and values as prices or error, if any.
// Returns error if base denom is not found in the token metadata.
// Sets the price to zero in case of failing to compute the price between base and quote but these being valid tokens.
func (t *tokensUseCase) getPricesForBaseDenom(ctx context.Context, baseDenom string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, pricingOptions ...domain.PricingOption) (map[string]osmomath.BigDec, error) {
	byQuoteDenomForGivenBaseResult := make(map[string]osmomath.BigDec, len(quoteDenoms))
	// Validate base denom is a valid denom
	// Return zeroes for all quotes if base denom is not found
	_, err := t.GetMetadataByChainDenom(baseDenom)
	if err != nil {
		for _, quoteDenom := range quoteDenoms {
			byQuoteDenomForGivenBaseResult[quoteDenom] = osmomath.ZeroBigDec()
		}
		return byQuoteDenomForGivenBaseResult, nil
	}

	// Get the pricing strategy
	pricingStrategy, ok := t.pricingStrategyMap[pricingSourceType]
	if !ok {
		return nil, fmt.Errorf("pricing strategy (%s) not found in the tokens use case", pricingStrategy)
	}

	defer func() {
		// Recover from panic if one occurred
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in getPricesForBaseDenom: %v", r)
		}
	}()

	for _, quoteDenom := range quoteDenoms {
		price, err := pricingStrategy.GetPrice(ctx, baseDenom, quoteDenom, pricingOptions...)
		if err != nil { // Check if we should fallback to another pricing source
			fallbackSourceType := pricingStrategy.GetFallbackStrategy(quoteDenom)
			if fallbackSourceType != domain.NoneSourceType {
				fallbackCounter.WithLabelValues(baseDenom, quoteDenom).Inc()
				fallbackPricingStrategy, ok := t.pricingStrategyMap[fallbackSourceType]
				if ok {
					price, err = fallbackPricingStrategy.GetPrice(ctx, baseDenom, quoteDenom, pricingOptions...)
				}
			}
		}

		if err != nil {
			price = osmomath.ZeroBigDec()

			// Increase prometheus counter
			pricingErrorCounter.WithLabelValues(baseDenom, quoteDenom, err.Error()).Inc()
		}

		byQuoteDenomForGivenBaseResult[quoteDenom] = price
	}

	return byQuoteDenomForGivenBaseResult, nil
}

func (t *tokensUseCase) getChainScalingFactorMut(precision int) (osmomath.Dec, bool) {
	result, ok := t.precisionScalingFactorMap.Load(precision)
	return result.(osmomath.Dec), ok
}

// UpdateAssetsAtHeightInterval updates assets at configured height interval.
// Internally, it calls LoadTokensFromChainRegistry as a goroutine.
func (t *tokensUseCase) UpdateAssetsAtHeightInterval(height uint64) {
	if height%uint64(t.updateAssetsHeightInterval) == 0 {
		go func() {
			t.LoadTokensFromChainRegistry() // nolint
		}()
	}
}

// LoadTokensFromChainRegistry loads tokens from the chain registry into TokensUsecase.
func (t *tokensUseCase) LoadTokensFromChainRegistry() error {
	tokens, err := GetTokensFromChainRegistry(t.chainRegistryAssetsFileURL)
	if err != nil {
		return err
	}

	t.LoadTokens(tokens)

	return nil
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
		token.IsUnlisted = asset.Preview
		token.CoingeckoID = asset.CoingeckoID
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
	metaData, ok := t.tokenMetadataByChainDenom.Load(chainDenom)
	return ok && !metaData.(domain.Token).IsUnlisted
}

// GetMinPoolLiquidityCap implements mvc.TokensUsecase.
func (t *tokensUseCase) GetMinPoolLiquidityCap(denomA, denomB string) (uint64, error) {
	// Get the pool denoms metadata
	poolDenomMetadataA, err := t.GetPoolDenomMetadata(denomA)
	if err != nil {
		return 0, err
	}

	poolDenomMetadataB, err := t.GetPoolDenomMetadata(denomB)
	if err != nil {
		return 0, err
	}

	// Get min liquidity
	minLiquidityCapBetweenTokens := osmomath.MinInt(poolDenomMetadataA.TotalLiquidityCap, poolDenomMetadataB.TotalLiquidityCap)

	if !minLiquidityCapBetweenTokens.IsUint64() {
		return 0, fmt.Errorf("min liquidity cap is greater than uint64, denomA: %s (%s), denomB: %s (%s)", denomA, poolDenomMetadataA.TotalLiquidity, denomB, poolDenomMetadataB.TotalLiquidity)
	}

	return minLiquidityCapBetweenTokens.Uint64(), nil
}

// IsValidPricingSource implements mvc.TokensUsecase.
func (t *tokensUseCase) IsValidPricingSource(pricingSource int) bool {
	ps := domain.PricingSourceType(pricingSource)
	return ps == domain.ChainPricingSourceType || ps == domain.CoinGeckoPricingSourceType
}

// GetCoingeckoIdByChainDenom implements mvc.TokensUsecase
func (t *tokensUseCase) GetCoingeckoIdByChainDenom(chainDenom string) (string, error) {
	if coingeckoId, found := t.coingeckoIds.Load(chainDenom); found {
		return coingeckoId.(string), nil
	} else {
		return "", fmt.Errorf("chain denom not found in chain registry")
	}
}
