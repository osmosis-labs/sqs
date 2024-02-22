package http

import (
	"context"
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

	_ "github.com/osmosis-labs/sqs/docs"
)

// TokensHandler  represent the httphandler for the router
type TokensHandler struct {
	TUsecase mvc.TokensUsecase
	RUsecase mvc.RouterUsecase
	logger   log.Logger
}

const (
	routerResource = "/tokens"

	defaultQuoteHumanDenom = "usdc"
)

func formatTokensResource(resource string) string {
	return routerResource + resource
}

// NewTokensHandler will initialize the pools/ resources endpoint
func NewTokensHandler(e *echo.Echo, ts mvc.TokensUsecase, ru mvc.RouterUsecase, logger log.Logger) error {
	handler := &TokensHandler{
		TUsecase: ts,
		RUsecase: ru,
		logger:   logger,
	}
	e.GET(formatTokensResource("/metadata"), handler.GetMetadata)
	e.GET(formatTokensResource("/prices"), handler.GetPrices)

	return nil
}

// @Summary Token Metadata
// @Description returns token metadata with chain denom, human denom, and precision.
// @Description For testnet, uses osmo-test-5 asset list. For mainnet, uses osmosis-1 asset list.
// @Description See `config.json` and `config-testnet.json` in root for details.
// @ID get-token-metadata
// @Produce  json
// @Param  denoms  path  string  false  "List of denoms where each can either be a human denom or a chain denom"
// @Success 200 {object} map[string]domain.Token "Success"
// @Router /tokens/metadata [get]
func (a *TokensHandler) GetMetadata(c echo.Context) (err error) {
	ctx := c.Request().Context()

	denomsStr := c.QueryParam("denoms")
	if len(denomsStr) == 0 {
		tokenMetadata, err := a.TUsecase.GetFullTokenMetadata(ctx)
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

		tokenMetadata, err := a.TUsecase.GetMetadataByChainDenom(ctx, denom)
		if err == nil {
			return c.JSON(http.StatusOK, tokenMetadata)
		}

		// If we fail to get metadata by chain denom, assume we are given a human denom and try to translate it.
		chainDenom, err := a.TUsecase.GetChainDenom(ctx, denom)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
		}

		// Repeat metadata retrieval
		tokenMetadata, err = a.TUsecase.GetMetadataByChainDenom(ctx, chainDenom)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
		}

		tokenMetadataResult[chainDenom] = tokenMetadata
	}

	return c.JSON(http.StatusOK, tokenMetadataResult)
}

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

	oneUSDC, err := a.getOneUnitChainScale(ctx, defaultQuoteHumanDenom)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
	}

	prices := make(map[string]map[string]any, len(baseDenoms))
	for i, denom := range baseDenoms {
		if isHumanDenoms {
			chainDenom, err := a.TUsecase.GetChainDenom(ctx, denom)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
			}

			baseDenoms[i] = chainDenom
		}

		quote, err := a.RUsecase.GetOptimalQuote(ctx, oneUSDC, baseDenoms[i])
		if err != nil {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
		}

		baseMetadata, err := a.TUsecase.GetMetadataByChainDenom(ctx, baseDenoms[i])
		if err != nil {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
		}

		scalingFactor, ok := a.TUsecase.GetChainScalingFactorMut(baseMetadata.Precision)
		if !ok {
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: fmt.Errorf("scaling factor for (%s) is not found", baseDenoms[i]).Error()})
		}

		chainPrice := osmomath.NewBigDecFromBigInt(oneUSDC.Amount.BigIntMut()).QuoMut(osmomath.NewBigDecFromBigInt(quote.GetAmountOut().BigIntMut()))

		precisionScalingFactor := osmomath.BigDecFromDec(scalingFactor.Quo(oneUSDC.Amount.ToLegacyDec()))

		currentPrice := chainPrice.MulMut(precisionScalingFactor)

		// from quote denom to price
		priceResultMap := make(map[string]any, 1)
		priceResultMap[oneUSDC.Denom] = currentPrice

		prices[baseDenoms[i]] = priceResultMap
	}

	return c.JSON(http.StatusOK, prices)
}

// returns one unit of the given human denom in chain scale. That is, converts to on-chain denom
// and applies precision scaling factor
func (a *TokensHandler) getOneUnitChainScale(ctx context.Context, humanDenom string) (sdk.Coin, error) {
	usdcDenom, err := a.TUsecase.GetChainDenom(ctx, humanDenom)
	if err != nil {
		return sdk.Coin{}, nil
	}

	usdcMetadata, err := a.TUsecase.GetMetadataByChainDenom(ctx, usdcDenom)
	if err != nil {
		return sdk.Coin{}, nil
	}

	scalingFactor, ok := a.TUsecase.GetChainScalingFactorMut(usdcMetadata.Precision)
	if !ok {
		return sdk.Coin{}, fmt.Errorf("scaling factor for (%s) is not found", humanDenom)
	}

	oneUSDC := sdk.NewCoin(usdcDenom, scalingFactor.TruncateInt())

	return oneUSDC, nil
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
