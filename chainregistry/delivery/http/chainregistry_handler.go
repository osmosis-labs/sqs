package http

import (
	"net/http"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"

	"github.com/labstack/echo/v4"
)

const chainregistryResource = "/chainregistry"

func formatRouterResource(resource string) string {
	return chainregistryResource + resource
}

// RouterHandler  represent the httphandler for the router
type ChainregistryHandler struct {
	usecase mvc.ChainregistryUsecase
	logger  log.Logger
}

// NewRouterHandler will initialize the pools/ resources endpoint
func NewChainregistryHandler(e *echo.Echo, chainregistryUseCase mvc.ChainregistryUsecase, logger log.Logger) {
	handler := &ChainregistryHandler{
		usecase: chainregistryUseCase,
		logger:  logger,
	}
	e.GET(formatRouterResource("/fee_tokens"), handler.GetFeeTokens)
}

func (a *ChainregistryHandler) GetFeeTokens(c echo.Context) (err error) {
	tokens, err := a.usecase.GetFeeTokens(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, api.GetFeeTokensResponse{
		FeeTokens: tokens,
	})
}
