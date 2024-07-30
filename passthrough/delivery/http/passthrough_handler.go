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

// PartialContent represent the partial content struct
type PartialContent struct {
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

	e.GET(formatPoolsResource("/portfolio-assets/:address"), handler.GetPortfolioAssetsByAddress)
}

// @Summary Returns portfolio assets associated with the given address.
// @Description The returned data represents the total value of the assets in the portfolio. Total value cap represents the total value of the assets in the portfolio.
// includes capitalization of user balances, value in locks, bonding or unbonding
// as well as the concentrated positions.
// Account coins result represents coins only from user balances (contrary to the total value cap).
// available = user-coin-balances + unclaimed-rewards + pooled - in-locks
// or
// available = total - staked - unstaking - in-locks
//
// pooled = gamm shares from user balances + value of CL positions
// @Produce  json
// @Success 200  struct  passthroughdomain.PortfolioAssetsResult  "Portfolio assets from user balances and capitalization of the entire account value"
// @Failure 206  struct  PartialContent  "Partial content and an rrror message"
// @Param address path string true "Wallet Address"
// @Router /passthrough/portfolio-assets/{address} [get]
func (a *PassthroughHandler) GetPortfolioAssetsByAddress(c echo.Context) error {
	address := c.Param("address")

	if address == "" {
		return c.JSON(http.StatusInternalServerError, ResponseError{Message: "invalid address: cannot be empty"})
	}

	portfolioAssetsResult, err := a.PUsecase.GetPortfolioAssets(c.Request().Context(), address)
	if err != nil {
		return c.JSON(http.StatusPartialContent, PartialContent{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, portfolioAssetsResult)
}
