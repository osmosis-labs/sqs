package usecase

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain/json"
)

type tokensUseCase struct {
	contextTimeout time.Duration

	metadataMapMu             sync.RWMutex
	tokenMetadataByChainDenom map[string]domain.Token

	denomMapMu           sync.RWMutex
	humanToChainDenomMap map[string]string

	// No mutex since we only expect reads to this shared resource and no writes.
	precisionScalingFactorMap map[int]osmomath.Dec
}

// Struct to represent the JSON structure
type AssetList struct {
	ChainName string `json:"chain_name"`
	Assets    []struct {
		Description string `json:"description"`
		DenomUnits  []struct {
			Denom    string `json:"denom"`
			Exponent int    `json:"exponent"`
		} `json:"denom_units"`
		Base     string        `json:"base"`
		Name     string        `json:"name"`
		Display  string        `json:"display"`
		Symbol   string        `json:"symbol"`
		Traces   []interface{} `json:"traces"`
		LogoURIs struct {
			PNG string `json:"png"`
			SVG string `json:"svg"`
		} `json:"logo_URIs"`
		CoingeckoID string   `json:"coingecko_id"`
		Keywords    []string `json:"keywords"`
	} `json:"assets"`
}

var _ mvc.TokensUsecase = &tokensUseCase{}

var (
	tenDec = osmomath.NewDec(10)
)

// NewTokensUsecase will create a new tokens use case object
func NewTokensUsecase(timeout time.Duration, chainRegistryAssetsFileURL string) (mvc.TokensUsecase, error) {
	tokenMetadataByChainDenom, err := getTokensFromChainRegistry(chainRegistryAssetsFileURL)
	if err != nil {
		return nil, err
	}

	// Create human denom to chain denom map
	humanToChainDenomMap := make(map[string]string, len(tokenMetadataByChainDenom))
	uniquePrecisionMap := make(map[int]struct{}, 0)

	for chainDenom, tokenMetadata := range tokenMetadataByChainDenom {
		// lower case human denom
		lowerCaseHumanDenom := strings.ToLower(tokenMetadata.HumanDenom)

		humanToChainDenomMap[lowerCaseHumanDenom] = chainDenom

		uniquePrecisionMap[tokenMetadata.Precision] = struct{}{}
	}

	// Precompute precision scaling factors
	precisionScalingFactors := make(map[int]osmomath.Dec, len(uniquePrecisionMap))
	for precision := range uniquePrecisionMap {
		precisionScalingFactors[precision] = tenDec.Power(uint64(precision))
	}

	return &tokensUseCase{
		contextTimeout: timeout,

		tokenMetadataByChainDenom: tokenMetadataByChainDenom,
		humanToChainDenomMap:      humanToChainDenomMap,
		precisionScalingFactorMap: precisionScalingFactors,

		metadataMapMu: sync.RWMutex{},
		denomMapMu:    sync.RWMutex{},
	}, nil
}

// GetChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) GetChainDenom(ctx context.Context, humanDenom string) (string, error) {
	humanDenomLowerCase := strings.ToLower(humanDenom)

	t.denomMapMu.RLock()
	defer t.denomMapMu.RUnlock()

	chainDenom, ok := t.humanToChainDenomMap[humanDenomLowerCase]
	if !ok {
		return "", fmt.Errorf("chain denom for human denom (%s) is not found", humanDenomLowerCase)
	}

	return chainDenom, nil
}

// GetMetadataByChainDenom implements mvc.TokensUsecase.
func (t *tokensUseCase) GetMetadataByChainDenom(ctx context.Context, denom string) (domain.Token, error) {
	t.metadataMapMu.RLock()
	defer t.metadataMapMu.RUnlock()
	token, ok := t.tokenMetadataByChainDenom[denom]
	if !ok {
		return domain.Token{}, fmt.Errorf("metadata for denom (%s) is not found", denom)
	}

	return token, nil
}

// GetFullTokenMetadata implements mvc.TokensUsecase.
func (t *tokensUseCase) GetFullTokenMetadata(ctx context.Context) (map[string]domain.Token, error) {
	t.metadataMapMu.RLock()
	defer t.metadataMapMu.RUnlock()

	// Do a copy of the cached metadata
	result := make(map[string]domain.Token, len(t.tokenMetadataByChainDenom))
	for denom, tokenMetadata := range t.tokenMetadataByChainDenom {
		result[denom] = tokenMetadata
	}

	return result, nil
}

// GetChainScalingFactorByDenomMut implements mvc.TokensUsecase.
func (t *tokensUseCase) GetChainScalingFactorByDenomMut(ctx context.Context, denom string) (osmomath.Dec, error) {
	denomMetadata, err := t.GetMetadataByChainDenom(ctx, denom)
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
func (t *tokensUseCase) GetPrices(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSource domain.PricingStrategy) (map[string]map[string]any, error) {
	byBaseDenomResult := make(map[string]map[string]any, len(baseDenoms))

	// For every base denom, create a map with quote denom prices.
	for _, baseDenom := range baseDenoms {
		byQuoteDenomForGivenBaseResult := make(map[string]any, len(quoteDenoms))

		// Given the current base denom, compute all of its prices with the quotes
		for _, quoteDenom := range quoteDenoms {
			price, err := pricingSource.GetPrice(ctx, baseDenom, quoteDenom)
			if err != nil {
				return nil, err
			}

			// Price for current base and quote
			byQuoteDenomForGivenBaseResult[quoteDenom] = price
		}

		// Finalize all prices for the current base and move on to the next base.
		byBaseDenomResult[baseDenom] = byQuoteDenomForGivenBaseResult
	}

	return byBaseDenomResult, nil
}

func (t *tokensUseCase) getChainScalingFactorMut(precision int) (osmomath.Dec, bool) {
	result, ok := t.precisionScalingFactorMap[precision]
	return result, ok
}

// getTokensFromChainRegistry fetches the tokens from the chain registry.
// It returns a map of tokens by chain denom.
func getTokensFromChainRegistry(chainRegistryAssetsFileURL string) (map[string]domain.Token, error) {
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
		chainDenom := ""

		if len(asset.DenomUnits) == 1 {
			// At time of script creation, only the following tokens have 1 denom unit with zero exponent:
			// one ibc/FE2CD1E6828EC0FAB8AF39BAC45BC25B965BA67CCBC50C13A14BD610B0D1E2C4 0
			// one ibc/52E12CF5CA2BB903D84F5298B4BFD725D66CAB95E09AA4FC75B2904CA5485FEB 0
			// one ibc/E27CD305D33F150369AB526AEB6646A76EC3FFB1A6CA58A663B5DE657A89D55D 0
			//
			// These seem as tokens that are not useful in routing so we silently skip them.
			continue
		}

		for _, denom := range asset.DenomUnits {
			if denom.Exponent == 0 {
				chainDenom = denom.Denom
			}

			if denom.Exponent > 0 {
				// There are edge cases where we have 3 denom exponents for a token.
				// We filter out the intermediate denom exponents and only use the human readable denom.
				if denom.Denom == "mluna" || denom.Denom == "musd" || denom.Denom == "msomm" || denom.Denom == "mkrw" || denom.Denom == "uarch" {
					continue
				}

				token.HumanDenom = denom.Denom
				token.Precision = denom.Exponent
			}
		}

		tokensByChainDenom[chainDenom] = token
	}

	return tokensByChainDenom, nil
}
