package http

import (
	"net/http"
	"net/url"

	"github.com/labstack/echo"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
)

// TokensHandler  represent the httphandler for the router
type TokensHandler struct {
	TUsecase mvc.TokensUsecase
	logger   log.Logger
}

const routerResource = "/tokens"

func formatTokensResource(resource string) string {
	return routerResource + resource
}

// NewTokensHandler will initialize the pools/ resources endpoint
func NewTokensHandler(e *echo.Echo, ts mvc.TokensUsecase, logger log.Logger) {
	handler := &TokensHandler{
		TUsecase: ts,
		logger:   logger,
	}
	e.GET(formatTokensResource("/metadata/:denom"), handler.GetMetadta)
}

// GetMetadata returns denom metadata
func (a *TokensHandler) GetMetadta(c echo.Context) (err error) {
	ctx := c.Request().Context()

	denom := c.Param("denom")

	denom, err = url.PathUnescape(denom)
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

	return c.JSON(http.StatusOK, tokenMetadata)
}
