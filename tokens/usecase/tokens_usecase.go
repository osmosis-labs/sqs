package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/domain/workerpool"
	"github.com/osmosis-labs/sqs/log"

	"github.com/osmosis-labs/osmosis/osmomath"
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

	// TokenRegistryLoader fetches tokens from the chain registry into the tokens use case
	tokenLoader domain.TokenRegistryLoader

	// Logger instance
	logger log.Logger
}

// Struct to represent the JSON structure
type AssetList struct {
	ChainName string `json:"chainName"`
	Assets    []struct {
		Name             string `json:"name"`
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

// NewTokensUsecase will create a new tokens use case object
func NewTokensUsecase(tokenMetadataByChainDenom map[string]domain.Token, updateAssetsHeightInterval int, logger log.Logger) *tokensUseCase {
	us := tokensUseCase{
		pricingStrategyMap:         map[domain.PricingSourceType]domain.PricingSource{},
		poolDenomMetaData:          sync.Map{},
		updateAssetsHeightInterval: updateAssetsHeightInterval,
		logger:                     logger,
	}

	us.LoadTokens(tokenMetadataByChainDenom)

	return &us
}

// SetTokenRegistryLoader sets the token registry loader for the tokens use case
func (t *tokensUseCase) SetTokenRegistryLoader(loader domain.TokenRegistryLoader) {
	t.tokenLoader = loader
}

// LoadTokensFunc is a function signature for LoadTokens.
type LoadTokensFunc func(tokenMetadataByChainDenom map[string]domain.Token)

// LoadTokens implements mvc.TokensUsecase.
func (t *tokensUseCase) LoadTokens(tokenMetadataByChainDenom map[string]domain.Token) {
	// Create human denom to chain denom map
	for chainDenom, tokenMetadata := range tokenMetadataByChainDenom {
		// lower case human denom
		lowerCaseHumanDenom := strings.ToLower(tokenMetadata.HumanDenom)

		t.humanToChainDenomMap.Store(lowerCaseHumanDenom, chainDenom)
		t.tokenMetadataByChainDenom.Store(chainDenom, tokenMetadata)

		t.chainDenoms.Store(chainDenom, struct{}{})

		t.coingeckoIds.Store(chainDenom, tokenMetadata.CoingeckoID)
	}
}

// UpdatePoolDenomMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) UpdatePoolDenomMetadata(poolDenomMetadata domain.PoolDenomMetaDataMap) {
	for chainDenom, tokenMetadata := range poolDenomMetadata {
		t.poolDenomMetaData.Store(chainDenom, tokenMetadata)
	}
}

// ClearPoolDenomMetadata implements mvc.TokensUsecase.
// WARNING: use with caution, this will clear all pool denom metadata
func (t *tokensUseCase) ClearPoolDenomMetadata() {
	t.poolDenomMetaData = sync.Map{}
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
		v, ok := chainDenom.(string)
		if ok {
			chainDenoms = append(chainDenoms, v)
		}
		return true
	})
	return t.GetPoolDenomsMetadata(chainDenoms)
}

// GetChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) GetChainDenom(humanDenom string) (string, error) {
	humanDenomLowerCase := strings.ToLower(humanDenom)

	chainDenom, ok := t.humanToChainDenomMap.Load(humanDenomLowerCase)
	if !ok {
		return "", ChainDenomForHumanDenomNotFoundError{ChainDenom: humanDenomLowerCase}
	}

	v, ok := chainDenom.(string)
	if !ok {
		return "", HumanDenomNotValidTypeError{HumanDenom: humanDenomLowerCase}
	}

	return v, nil
}

// GetMetadataByChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) GetMetadataByChainDenom(denom string) (domain.Token, error) {
	token, ok := t.tokenMetadataByChainDenom.Load(denom)
	if !ok {
		return domain.Token{}, MetadataForChainDenomNotFoundError{ChainDenom: denom}
	}

	v, ok := token.(domain.Token)
	if !ok {
		return domain.Token{}, MetadataForChainDenomNotValidTypeError{ChainDenom: denom}
	}

	return v, nil
}

// GetFullTokenMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) GetFullTokenMetadata() (map[string]domain.Token, error) {
	// Do a copy of the cached metadata
	var err error
	result := make(map[string]domain.Token)
	t.tokenMetadataByChainDenom.Range(func(denom, token any) bool {
		d, ok := denom.(string)
		if !ok {
			err = DenomNotValidTypeError{Denom: denom}
			return false
		}

		t, ok := token.(domain.Token)
		if !ok {
			err = TokenNotValidTypeError{Token: token}
			return false
		}

		result[d] = t

		return true
	})
	return result, err
}

// GetChainScalingFactorByDenomMut implements mvc.TokensUsecase.
func (t *tokensUseCase) GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error) {
	denomMetadata, err := t.GetMetadataByChainDenom(denom)
	if err != nil {
		return osmomath.Dec{}, err
	}

	scalingFactor, ok := getPrecisionScalingFactorMut(denomMetadata.Precision)
	if !ok {
		return osmomath.Dec{}, ScalingFactorForPrecisionNotFoundError{
			Precision: denomMetadata.Precision,
			Denom:     denom,
		}
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
	defer basePriceDispatcher.Stop()

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
				domain.SQSPricingFallbackCounter.WithLabelValues(baseDenom, quoteDenom).Inc()
				fallbackPricingStrategy, ok := t.pricingStrategyMap[fallbackSourceType]
				if ok {
					price, err = fallbackPricingStrategy.GetPrice(ctx, baseDenom, quoteDenom, pricingOptions...)
				}
			}
		}

		if err != nil {
			price = osmomath.ZeroBigDec()

			// Increase prometheus counter
			domain.SQSPricingErrorCounter.WithLabelValues(baseDenom, quoteDenom, err.Error()).Inc()
		}

		byQuoteDenomForGivenBaseResult[quoteDenom] = price
	}

	return byQuoteDenomForGivenBaseResult, nil
}

// UpdateAssetsAtHeightIntervalSync updates assets at configured height interval.
func (t *tokensUseCase) UpdateAssetsAtHeightIntervalSync(height uint64) error {
	if height%uint64(t.updateAssetsHeightInterval) == 0 {
		if err := t.tokenLoader.FetchAndUpdateTokens(); err != nil {
			return err
		}
	}
	return nil
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
	if !ok {
		return false
	}

	v, ok := metaData.(domain.Token)
	if !ok {
		return false
	}

	// is valid only if token is found and is not unlisted
	return !v.IsUnlisted
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
	coingeckoId, found := t.coingeckoIds.Load(chainDenom)
	if !found {
		return "", ChainDenomNotFoundInChainRegistryError{}
	}

	v, ok := coingeckoId.(string)
	if !ok {
		return "", CoingeckoIDNotValidTypeError{
			CoingeckoID: coingeckoId,
			Denom:       chainDenom,
		}
	}

	return v, nil
}
