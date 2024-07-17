package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

// ResponseError represent the response error struct
type ResponseError struct {
	Message string `json:"message"`
}

// PassthroughHandler is the http handler for passthrough use case
type PassthroughHandler struct {
	PUsecase mvc.PassthroughUsecase
}

const resourcePrefix = "/passthrough"

func formatPoolsResource(resource string) string {
	return resourcePrefix + resource
}

// NewPassthroughHandler will initialize the pools/ resources endpoint
func NewPassthroughHandler(e *echo.Echo, ptu mvc.PassthroughUsecase) {
	handler := &PassthroughHandler{
		PUsecase: ptu,
	}

	e.GET(formatPoolsResource("/portfolio-assets/:address"), handler.GetAccountCoinsTotal)
}

// GetAccountCoinsTotal adds an API handler to get total assets data
func (a *PassthroughHandler) GetAccountCoinsTotal(c echo.Context) error {
	address := c.Param("address")

	if address == "" {
		return c.JSON(http.StatusInternalServerError, ResponseError{Message: "invalid address: cannot be empty"})
	}

	portfolioAssetsResult, err := a.PUsecase.GetPortfolioAssets(c.Request().Context(), address)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, portfolioAssetsResult)
}
