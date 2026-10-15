package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/domain/workerpool"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"

	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
)

const (
	// Max number of workers to fetch prices concurrently
	// TODO: move to config
	maxNumWorkes = 10
)

type TokensUseCase struct {
	// Can be considered for merge with humanToChainDenomMap in the future.
	tokenMetadataByChainDenom sync.Map // domain.Token
	humanToChainDenomMap      sync.Map // string, keyed by lower case human denom
	humanExactToChainDenomMap sync.Map // string, keyed by human denom as listed
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

	// Default quote denom to use for pricing
	defaultQuoteDenom string

	// TokenPriceFetcher is used for fetching token prices.
	tokenPriceFetcher domain.TokensPriceFetcher

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
		Decimals         *int   `json:"decimals"`
		CoingeckoID      string `json:"coingeckoId"`
		Preview          bool   `json:"preview"`
	} `json:"assets"`
}

// Define a result struct to hold the base denom and prices for each possible quote denom or error
type priceResults struct {
	baseDenom string
	prices    map[string]domain.PriceResult
	err       error
}

var _ mvc.TokensUsecase = &TokensUseCase{}

// NewTokensUsecase will create a new tokens use case object
func NewTokensUsecase(tokenMetadataByChainDenom map[string]domain.Token, updateAssetsHeightInterval int, logger log.Logger) *TokensUseCase {
	us := &TokensUseCase{
		pricingStrategyMap:         map[domain.PricingSourceType]domain.PricingSource{},
		poolDenomMetaData:          sync.Map{},
		updateAssetsHeightInterval: updateAssetsHeightInterval,
		logger:                     logger,
	}

	us.LoadTokens(tokenMetadataByChainDenom)

	us.tokenPriceFetcher = us

	return us
}

// SetTokenRegistryLoader sets the token registry loader for the tokens use case
func (t *TokensUseCase) SetTokenRegistryLoader(loader domain.TokenRegistryLoader) {
	t.tokenLoader = loader
}

// SetDefaultQuoteDenom sets the default quote denom for the tokens use case
func (t *TokensUseCase) SetDefaultQuoteDenom(denom string) {
	t.defaultQuoteDenom = denom
}

// LoadTokensFunc is a function signature for LoadTokens.
type LoadTokensFunc func(tokenMetadataByChainDenom map[string]domain.Token)

// LoadTokens implements mvc.TokensUsecase.
// Several assets can share a human denom (exactly or case-insensitively). Resolution is
// deterministic: listed assets win over preview assets, then the lexicographically smallest
// chain denom wins. Exact-case lookups are tried before case-insensitive ones in GetChainDenom.
func (t *TokensUseCase) LoadTokens(tokenMetadataByChainDenom map[string]domain.Token) {
	// Iterate in sorted order so that ties resolve the same way on every instance.
	chainDenoms := make([]string, 0, len(tokenMetadataByChainDenom))
	for chainDenom := range tokenMetadataByChainDenom {
		chainDenoms = append(chainDenoms, chainDenom)
	}
	sort.Strings(chainDenoms)

	exactWinners := make(map[string]string, len(chainDenoms))
	lowerCaseWinners := make(map[string]string, len(chainDenoms))
	lowerCaseCandidates := make(map[string][]string, len(chainDenoms))

	for _, chainDenom := range chainDenoms {
		tokenMetadata := tokenMetadataByChainDenom[chainDenom]

		// lower case human denom
		lowerCaseHumanDenom := strings.ToLower(tokenMetadata.HumanDenom)

		setHumanDenomWinner(exactWinners, tokenMetadata.HumanDenom, chainDenom, tokenMetadataByChainDenom)
		setHumanDenomWinner(lowerCaseWinners, lowerCaseHumanDenom, chainDenom, tokenMetadataByChainDenom)
		lowerCaseCandidates[lowerCaseHumanDenom] = append(lowerCaseCandidates[lowerCaseHumanDenom], chainDenom)

		t.tokenMetadataByChainDenom.Store(chainDenom, tokenMetadata)

		t.chainDenoms.Store(chainDenom, struct{}{})

		t.coingeckoIds.Store(chainDenom, tokenMetadata.CoingeckoID)
	}

	for humanDenom, chainDenom := range exactWinners {
		t.humanExactToChainDenomMap.Store(humanDenom, chainDenom)
	}

	for lowerCaseHumanDenom, chainDenom := range lowerCaseWinners {
		t.humanToChainDenomMap.Store(lowerCaseHumanDenom, chainDenom)

		if candidates := lowerCaseCandidates[lowerCaseHumanDenom]; len(candidates) > 1 && t.logger != nil {
			t.logger.Warn("human denom resolves to multiple chain denoms", zap.String("human_denom", lowerCaseHumanDenom), zap.Strings("chain_denoms", candidates), zap.String("resolved_case_insensitive", chainDenom))
		}
	}
}

// setHumanDenomWinner records chainDenom as the resolution for humanDenom unless an already recorded
// chain denom is preferred. Chain denoms must be visited in sorted order so that ties are deterministic.
func setHumanDenomWinner(winners map[string]string, humanDenom string, chainDenom string, tokenMetadataByChainDenom map[string]domain.Token) {
	current, ok := winners[humanDenom]
	if !ok || (tokenMetadataByChainDenom[current].IsUnlisted && !tokenMetadataByChainDenom[chainDenom].IsUnlisted) {
		winners[humanDenom] = chainDenom
	}
}

// StoreTokenStateFiles implements mvc.TokensUsecase.
func (t *TokensUseCase) StoreTokensStateFiles() error {
	tokensMetadata, err := t.GetFullTokenMetadata()
	if err != nil {
		return err
	}

	if err = parsing.StoreTokensMetadata(tokensMetadata, "tokens.json"); err != nil {
		return err
	}

	poolDenomMetaData := t.GetFullPoolDenomMetadata()
	if err = parsing.StorePoolDenomMetaData(poolDenomMetaData, "pool_denom_metadata.json"); err != nil {
		return err
	}

	return nil
}

// LoadTokenStateFiles implements mvc.TokensUsecase.
func (t *TokensUseCase) LoadTokensStateFiles() error {
	tokensMetadata, err := parsing.ReadTokensMetadata("./state/tokens.json")
	if err != nil {
		return err
	}

	t.LoadTokens(tokensMetadata)

	poolDenomMetaData, err := parsing.ReadPoolDenomsMetaData("./state/pool_denom_metadata.json")
	if err != nil {
		return err
	}

	t.UpdatePoolDenomMetadata(poolDenomMetaData)

	return nil
}

// UpdatePoolDenomMetadata implements mvc.TokensUsecase.
func (t *TokensUseCase) UpdatePoolDenomMetadata(poolDenomMetadata domain.PoolDenomMetaDataMap) {
	for chainDenom, tokenMetadata := range poolDenomMetadata {
		t.poolDenomMetaData.Store(chainDenom, tokenMetadata)
	}
}

// ClearPoolDenomMetadata implements mvc.TokensUsecase.
// WARNING: use with caution, this will clear all pool denom metadata
func (t *TokensUseCase) ClearPoolDenomMetadata() {
	t.poolDenomMetaData = sync.Map{}
}

// GetPoolLiquidityCap implements mvc.TokensUsecase.
func (t *TokensUseCase) GetPoolLiquidityCap(chainDenom string) (osmomath.Int, error) {
	poolDenomMetadata, err := t.GetPoolDenomMetadata(chainDenom)
	if err != nil {
		return osmomath.Int{}, err
	}
	return poolDenomMetadata.TotalLiquidity, nil
}

// GetPoolDenomMetadata implements mvc.TokensUsecase.
func (t *TokensUseCase) GetPoolDenomMetadata(chainDenom string) (domain.PoolDenomMetaData, error) {
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
func (t *TokensUseCase) GetPoolDenomsMetadata(chainDenoms []string) domain.PoolDenomMetaDataMap {
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
func (t *TokensUseCase) GetFullPoolDenomMetadata() domain.PoolDenomMetaDataMap {
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
func (t *TokensUseCase) GetChainDenom(humanDenom string) (string, error) {
	humanDenomLowerCase := strings.ToLower(humanDenom)

	// Prefer an exact-case match so that case-colliding symbols (e.g. aUSD vs AUSD) can be told apart.
	chainDenom, ok := t.humanExactToChainDenomMap.Load(humanDenom)
	if !ok {
		chainDenom, ok = t.humanToChainDenomMap.Load(humanDenomLowerCase)
	}
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
func (t *TokensUseCase) GetMetadataByChainDenom(denom string) (domain.Token, error) {
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
func (t *TokensUseCase) GetFullTokenMetadata() (map[string]domain.Token, error) {
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
func (t *TokensUseCase) GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error) {
	denomMetadata, err := t.GetMetadataByChainDenom(denom)
	if err != nil {
		return osmomath.Dec{}, err
	}

	scalingFactor, ok := getPrecisionScalingFactorImmutable(denomMetadata.Precision)
	if !ok {
		return osmomath.Dec{}, ScalingFactorForPrecisionNotFoundError{
			Precision: denomMetadata.Precision,
			Denom:     denom,
		}
	}

	return scalingFactor, nil
}

// GetPrices implements pricing.PricingStrategy.
func (t *TokensUseCase) GetPrices(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
	byBaseDenomResult := make(domain.PricesResult, len(baseDenoms))

	numWorkers := min(len(baseDenoms), maxNumWorkes)

	basePriceDispatcher := workerpool.NewDispatcher[priceResults](numWorkers)
	go basePriceDispatcher.Run()
	defer basePriceDispatcher.Stop()

	// For every base denom, create a map with quote denom prices.
	for _, baseDenom := range baseDenoms {
		basePriceDispatcher.JobQueue <- workerpool.Job[priceResults]{
			Task: func() (result priceResults, err error) {
				defer func() {
					// Recover from panic if one occurred
					if r := recover(); r != nil {
						result = priceResults{
							baseDenom: baseDenom,
							err:       fmt.Errorf("panic in GetPrices for base denom (%s): %v", baseDenom, r),
						}
					}
				}()

				// A denom still queued when the request is abandoned or times out is not priced.
				if err := ctx.Err(); err != nil {
					return priceResults{baseDenom: baseDenom, err: err}, nil
				}

				prices, err := t.getPricesForBaseDenom(ctx, baseDenom, quoteDenoms, pricingSourceType, opts...)
				if err != nil {
					t.logger.Error("failed to get prices for base denom", zap.String("baseDenom", baseDenom), zap.Error(err))
				}

				return priceResults{
					baseDenom: baseDenom,
					prices:    prices,
					err:       err,
				}, nil
			},
		}
	}

	// Read every result before returning, even after an error, so that no worker
	// stays blocked on the unbuffered result queue.
	var firstErr error
	for range baseDenoms {
		result := <-basePriceDispatcher.ResultQueue

		if result.Result.err != nil {
			if firstErr == nil {
				firstErr = result.Result.err
			}
			continue
		}
		byBaseDenomResult[result.Result.baseDenom] = result.Result.prices
	}

	if firstErr != nil {
		return nil, firstErr
	}

	return byBaseDenomResult, nil
}

// getPricesForBaseDenom fetches all prices for base denom given a slice of quotes and pricing options.
// Pricing options determine whether to recompute prices or use the cache as well as the desired source of prices.
// Returns a map with keys as quotes and values as prices or error, if any.
// A quote denom is omitted from the result when no positive price can be computed for it,
// and all quotes are omitted when the base denom is not found in the token metadata.
// Callers must treat a missing entry as "no price", never as zero.
func (t *TokensUseCase) getPricesForBaseDenom(ctx context.Context, baseDenom string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, pricingOptions ...domain.PricingOption) (result map[string]domain.PriceResult, err error) {
	byQuoteDenomForGivenBaseResult := make(map[string]domain.PriceResult, len(quoteDenoms))
	// Validate base denom is a valid denom
	// Omit all quotes if base denom is not found
	if _, err := t.GetMetadataByChainDenom(baseDenom); err != nil {
		return byQuoteDenomForGivenBaseResult, nil
	}

	// Get the pricing strategy
	pricingStrategy, ok := t.pricingStrategyMap[pricingSourceType]
	if !ok {
		return nil, fmt.Errorf("pricing strategy (%d) not found in the tokens use case", pricingSourceType)
	}

	defer func() {
		// Recover from panic if one occurred
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("panic in getPricesForBaseDenom: %v", r)
		}
	}()

	for _, quoteDenom := range quoteDenoms {
		price, routes, err := pricingStrategy.GetPrice(ctx, baseDenom, quoteDenom, pricingOptions...)
		if err == nil {
			err = validatePrice(price)
		}
		// A request that was abandoned or timed out is neither priced from the fallback source
		// nor answered with the pair left out as unpriced.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if err != nil { // Check if we should fallback to another pricing source
			fallbackSourceType := pricingStrategy.GetFallbackStrategy(quoteDenom)
			if fallbackSourceType != domain.NoneSourceType {
				t.logger.Info(domain.SQSPricingFallbackCounterMetricName, zap.String("baseDenom", baseDenom), zap.String("quoteDenom", quoteDenom))
				domain.SQSPricingFallbackCounter.Inc()
				fallbackPricingStrategy, ok := t.pricingStrategyMap[fallbackSourceType]
				if ok {
					price, routes, err = fallbackPricingStrategy.GetPrice(ctx, baseDenom, quoteDenom, pricingOptions...)
					if err == nil {
						err = validatePrice(price)
					}
				}
			}
		}

		if err != nil {
			// Omit the quote rather than serving a zero price.
			// Increase prometheus counter
			t.logger.Error(domain.SQSPricingErrorCounterMetricName, zap.String("baseDenom", baseDenom), zap.String("quoteDenom", quoteDenom), zap.Error(err))
			domain.SQSPricingErrorCounter.Inc()
			continue
		}

		byQuoteDenomForGivenBaseResult[quoteDenom] = domain.NewPriceResult(price, routes)
	}

	return byQuoteDenomForGivenBaseResult, nil
}

// validatePrice returns an error if the price is nil, zero or negative.
func validatePrice(price osmomath.BigDec) error {
	if price.IsNil() || !price.IsPositive() {
		return fmt.Errorf("price is not positive")
	}
	return nil
}

// CalcSpotPrice calculates the spot price between two denoms.
// Spot price is derived from the chain prices across all pools between the two denoms and
// normalized by the scaling factor derived from CalcScalingFactor.
// Formula is: spotPrice = (base denom price / quote denom price) * scaling factor
// Returns error if any of the prices is not found or if the quote denom price is zero.
func (t *TokensUseCase) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	// Get the price of base denom in the default quote denom
	basePrices, err := t.tokenPriceFetcher.GetPrices(ctx, []string{baseDenom}, []string{t.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Get the price of base denom in the default quote denom
	quotePrices, err := t.tokenPriceFetcher.GetPrices(ctx, []string{quoteDenom}, []string{t.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	basePrice := basePrices[baseDenom][t.defaultQuoteDenom]
	if basePrice.Price.IsNil() {
		return osmomath.BigDec{}, fmt.Errorf("base denom price is nil")
	}

	quotePrice := quotePrices[quoteDenom][t.defaultQuoteDenom]
	if quotePrice.Price.IsNil() {
		return osmomath.BigDec{}, fmt.Errorf("quote denom price is nil")
	}

	if quotePrice.Price.IsZero() {
		return osmomath.BigDec{}, fmt.Errorf("quote denom price is zero")
	}

	// Calculate raw spot price
	p := basePrice.Price.Quo(quotePrice.Price)

	scaleFactor, err := t.CalcScalingFactor(baseDenom, quoteDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Normalize using scaling factor
	p = p.MulMut(scaleFactor)

	return p, nil
}

// CalcScalingFactor calculates the scaling factor between two denoms.
// Formula is: scaleFactor = (10^quote_decimals) / (10^base_decimals)
// Returns error if any of the denoms is not found in the token metadata.
func (t *TokensUseCase) CalcScalingFactor(baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	// Retrieve metadata for base denom
	baseDenomMetadata, err := t.GetMetadataByChainDenom(baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Retrieve metadata for quote denom
	quoteDenomMetadata, err := t.GetMetadataByChainDenom(quoteDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	tenDec := osmomath.NewBigDec(10)

	// Calculate scaling factors based on token precision
	baseScale := tenDec.PowerInteger(uint64(baseDenomMetadata.Precision))
	quoteScale := tenDec.PowerInteger(uint64(quoteDenomMetadata.Precision))

	// Calculate scale factor between quote and base
	return quoteScale.Quo(baseScale), nil
}

// UpdateAssetsAtHeightIntervalSync updates assets at configured height interval.
func (t *TokensUseCase) UpdateAssetsAtHeightIntervalSync(height uint64) error {
	if height%uint64(t.updateAssetsHeightInterval) == 0 {
		if err := t.tokenLoader.FetchAndUpdateTokens(); err != nil {
			return err
		}
	}
	return nil
}

// GetSpotPriceScalingFactorByDenomMut implements mvc.TokensUsecase.
func (t *TokensUseCase) GetSpotPriceScalingFactorByDenom(baseDenom string, quoteDenom string) (osmomath.Dec, error) {
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
func (t *TokensUseCase) RegisterPricingStrategy(source domain.PricingSourceType, strategy domain.PricingSource) {
	t.pricingStrategyMap[source] = strategy
}

// IsValidChainDenom implements mvc.TokensUsecase.
func (t *TokensUseCase) IsValidChainDenom(chainDenom string) bool {
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

// GetMinPoolEffectiveLiquidityCap implements mvc.TokensUsecase.
func (t *TokensUseCase) GetMinPoolEffectiveLiquidityCap(denomA, denomB string) (uint64, error) {
	// Get the pool denoms metadata
	poolDenomMetadataA, err := t.GetPoolDenomMetadata(denomA)
	if err != nil {
		return 0, err
	}

	poolDenomMetadataB, err := t.GetPoolDenomMetadata(denomB)
	if err != nil {
		return 0, err
	}

	if poolDenomMetadataA.TotalEffectiveLiquidityCap.IsNil() || poolDenomMetadataB.TotalEffectiveLiquidityCap.IsNil() {
		return 0, fmt.Errorf("effective capitalizations are not computed")
	}

	// Get min liquidity
	minLiquidityCapBetweenTokens := osmomath.MinInt(poolDenomMetadataA.TotalEffectiveLiquidityCap, poolDenomMetadataB.TotalEffectiveLiquidityCap)
	if !minLiquidityCapBetweenTokens.IsUint64() {
		return 0, fmt.Errorf("min liquidity cap is greater than uint64, denomA: %s (%s), denomB: %s (%s)", denomA, poolDenomMetadataA.TotalLiquidity, denomB, poolDenomMetadataB.TotalLiquidity)
	}

	return minLiquidityCapBetweenTokens.Uint64(), nil
}

// GetMinPoolLiquidityCap implements mvc.TokensUsecase.
func (t *TokensUseCase) GetMinPoolLiquidityCap(denomA, denomB string) (uint64, error) {
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
func (t *TokensUseCase) IsValidPricingSource(pricingSource int) bool {
	ps := domain.PricingSourceType(pricingSource)
	return ps == domain.ChainPricingSourceType || ps == domain.CoinGeckoPricingSourceType
}

// GetCoingeckoIdByChainDenom implements mvc.TokensUsecase
func (t *TokensUseCase) GetCoingeckoIdByChainDenom(chainDenom string) (string, error) {
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
