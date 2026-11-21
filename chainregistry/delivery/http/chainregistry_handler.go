package http

import (
	"net/http"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"

	"github.com/labstack/echo/v4"
)

const chainregistryResource = "/chainregistry"

// ChainregistryHandler represents the HTTP handler for the chain registry.
type ChainregistryHandler struct {
	usecase mvc.ChainregistryUsecase
}

// NewChainregistryHandler initializes the chainregistry HTTP endpoints.
func NewChainregistryHandler(e *echo.Echo, chainregistryUseCase mvc.ChainregistryUsecase) {
	handler := &ChainregistryHandler{
		usecase: chainregistryUseCase,
	}
	e.GET(chainregistryResource+"/fee_tokens", handler.GetFeeTokens)
}

func (a *ChainregistryHandler) GetFeeTokens(c echo.Context) (err error) {
	if err := c.Request().Context().Err(); err != nil {
		return c.JSON(http.StatusServiceUnavailable, domain.ResponseError{Message: "request cancelled"})
	}

	tokens, err := a.usecase.GetFeeTokens(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, api.GetFeeTokensResponse{
		FeeTokens: tokens,
	})
}
