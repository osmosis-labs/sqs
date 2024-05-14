package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"

	_ "github.com/osmosis-labs/sqs/docs"
)

// TokensHandler  represent the httphandler for the router
type TokensHandler struct {
	TUsecase mvc.TokensUsecase
	RUsecase mvc.RouterUsecase

	logger log.Logger
}

const (
	routerResource = "/tokens"

	// TODO: move to config
	defaultQuoteHumanDenom = "usdc"
	defaultPricingSource   = domain.ChainPricingSourceType
)

var (
	defaultQuoteChainDenom string
)

func formatTokensResource(resource string) string {
	return routerResource + resource
}

// NewTokensHandler will initialize the pools/ resources endpoint
func NewTokensHandler(e *echo.Echo, pricingConfig domain.PricingConfig, ts mvc.TokensUsecase, ru mvc.RouterUsecase, logger log.Logger) (err error) {
	handler := &TokensHandler{
		TUsecase: ts,
		RUsecase: ru,

		logger: logger,
	}

	e.GET(formatTokensResource("/metadata"), handler.GetMetadata)
	e.GET(formatTokensResource("/prices"), handler.GetPrices)
	e.GET(formatTokensResource("/usd-price-test"), handler.GetUSDPriceTest)
	e.POST(formatTokensResource("/store-state"), handler.StoreTokensStateInFiles)

	defaultQuoteChainDenom, err = ts.GetChainDenom(defaultQuoteHumanDenom)
	if err != nil {
		return err
	}
	return nil
}

// @Summary Token Metadata
// @Description returns token metadata with chain denom, human denom, and precision.
// @Description For testnet, uses osmo-test-5 asset list. For mainnet, uses osmosis-1 asset list.
// @Description See `config.json` and `config-testnet.json` in root for details.
// @ID get-token-metadata
// @Produce  json
// @Param  denoms  query  string  false  "List of denoms where each can either be a human denom or a chain denom"
// @Success 200 {object} map[string]domain.Token "Success"
// @Router /tokens/metadata [get]
func (a *TokensHandler) GetMetadata(c echo.Context) (err error) {
	denomsStr := c.QueryParam("denoms")
	if len(denomsStr) == 0 {
		tokenMetadata, err := a.TUsecase.GetFullTokenMetadata()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
		}
		return c.JSON(http.StatusOK, tokenMetadata)
	}

	denoms := strings.Split(denomsStr, ",")

	tokenMetadataResult := make(map[string]domain.Token, len(denoms))

	for _, denom := range denoms {
		denom, err := url.PathUnescape(denom)
		if err != nil {
			return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
		}

		if err := sdk.ValidateDenom(denom); err != nil {
			return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
		}

		tokenMetadata, err := a.TUsecase.GetMetadataByChainDenom(denom)
		if err == nil {
			return c.JSON(http.StatusOK, tokenMetadata)
		}

		// If we fail to get metadata by chain denom, assume we are given a human denom and try to translate it.
		chainDenom, err := a.TUsecase.GetChainDenom(denom)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
		}

		// Repeat metadata retrieval
		tokenMetadata, err = a.TUsecase.GetMetadataByChainDenom(chainDenom)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
		}

		tokenMetadataResult[chainDenom] = tokenMetadata
	}

	return c.JSON(http.StatusOK, tokenMetadataResult)
}

// @Summary Get prices
// @Description Given a list of base denominations, this endpoint returns the spot price with a system-configured quote denomination.
// If the pricing source is set to "chain" (0), it will first check the **chain** pricing cache for the price quote. If it exists, it will return it. Otherwise, it will compute the pricing on-demand if the quote is non-usdc.
// If the pricing source is set to "coingecko" (1), it will look for the price quote in the **coingecko** pricing cache. If it exists, it will return it. Otherwise, it will fetch the price from the Coingecko API endpoint and store it in the cache with an expiration time specified in the config.json file.
// If the token price is not available from the chain pricing source for any reason, it will fallback to the Coingecko pricing source if the quote denomination (human or chain) is usdc.
// See also: https://github.com/osmosis-labs/sqs/blob/de34d172f95b221217967799f233c52181cfa07e/README.md#pricing
// @Accept  json
// @Produce  json
// @Param   base          query     string  true  "Comma-separated list of base denominations (human-readable or chain format based on humanDenoms parameter)"
// @Param   humanDenoms   query     bool    false "Specify true if input denominations are in human-readable format; defaults to false"
// @Param	pricingSource query		int     false "Specify the pricing source. Values can be 0 (chain) or 1 (coingecko); default to 0 (chain)
// @Success 200 {object} map[string]map[string]string "A map where each key is a base denomination (on-chain format), containing another map with a key as the quote denomination (on-chain format) and the value as the spot price."
// @Router /tokens/prices [get]
func (a *TokensHandler) GetPrices(c echo.Context) (err error) {
	ctx := c.Request().Context()

	baseDenomsStr := c.QueryParam("base")
	baseDenoms, err := validateDenomsParam(baseDenomsStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	isHumanDenomsStr := c.QueryParam("humanDenoms")
	isHumanDenoms := false
	if len(isHumanDenomsStr) > 0 {
		isHumanDenoms, err = strconv.ParseBool(isHumanDenomsStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
		}
	}

	// Check if the provided denoms (which can be human or chain) are valid and existing in the asset list
	// If human denoms, convert to chain denoms
	// If chain denoms, validate if they are valid chain denoms
	// If any of the denoms are invalid, reject the entire request and return an error
	if isHumanDenoms {
		for i, baseDenom := range baseDenoms {
			baseDenoms[i], err = a.TUsecase.GetChainDenom(baseDenom)
			if err != nil {
				return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
			}
		}
	} else {
		for _, baseDenom := range baseDenoms {
			if !a.TUsecase.IsValidChainDenom(baseDenom) {
				return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: fmt.Sprintf("invalid chain denom: %s", baseDenom)})
			}
		}
	}

	// Check if pricingSource param is valid
	var pricingSource domain.PricingSourceType
	pricingSourceParam := c.QueryParam("pricingSource")
	if pricingSourceParam == "" {
	         // By default, if no source is explicitly provided, we use a chain pricing source. 
		pricingSource = domain.ChainPricingSourceType
	} else {
	        // Otherwise, we attempt to parse the given pricing source.
		pricingSourceInt, err := strconv.Atoi(pricingSourceParam)
		if err != nil || !a.TUsecase.IsValidPricingSource(pricingSourceInt) {
			return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: fmt.Sprintf("invalid pricing source: %s", pricingSourceParam)})
		}
		pricingSource = domain.PricingSourceType(pricingSourceInt)
	}

	prices, err := a.TUsecase.GetPrices(ctx, baseDenoms, []string{defaultQuoteChainDenom}, pricingSource)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
	}
	return c.JSON(http.StatusOK, prices)
}

// validateDenomsParam validates the denoms param string
// returns a denom slice if validation passes. Error otherwise
func validateDenomsParam(denomsStr string) ([]string, error) {
	if len(denomsStr) == 0 {
		return nil, errors.New("denoms input must be non-empty")
	}

	denoms := strings.Split(denomsStr, ",")

	for _, denom := range denoms {
		denom, err := url.PathUnescape(denom)
		if err != nil {
			return nil, err
		}

		if err := sdk.ValidateDenom(denom); err != nil {
			return nil, err
		}
	}

	return denoms, nil
}

// This mock endpoint is exposed for a data-pipelines hiring assignment.
// It is not meant for use in production.
func (a *TokensHandler) GetUSDPriceTest(c echo.Context) (err error) {
	denomsStr := c.QueryParam("denoms")
	denoms, err := validateDenomsParam(denomsStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	prices := map[string]osmomath.Dec{}

	for _, denom := range denoms {
		// TiA
		if denom == "ibc/D79E7D83AB399BFFF93433E54FAA480C191248FC556924A2A8351AE2638B3877" {
			prices[denom] = osmomath.MustNewDecFromStr("13.5")
			// milkTIA
		} else if denom == "factory/osmo1f5vfcph2dvfeqcqkhetwv75fda69z7e5c2dldm3kvgj23crkv6wqcn47a0/umilkTIA" {
			prices[denom] = osmomath.MustNewDecFromStr("13.7")
		} else {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: fmt.Errorf("unsupported denom (%s)", denom).Error()})
		}
	}

	return c.JSON(http.StatusOK, prices)
}

func (a *TokensHandler) StoreTokensStateInFiles(c echo.Context) error {
	tokensMetadata, err := a.TUsecase.GetFullTokenMetadata()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
	}

	err = parsing.StoreTokensMetadata(tokensMetadata, "tokens.json")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, "Tokens metadata state stored in files")
}
