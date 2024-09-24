package http

import (
	"net/http"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	_ "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/orderbook/types"

	"github.com/labstack/echo/v4"

	"go.uber.org/zap"
)

// PassthroughHandler is the http handler for passthrough use case
type PassthroughHandler struct {
	PUsecase mvc.PassthroughUsecase
	OUsecase mvc.OrderBookUsecase
	Logger   log.Logger
}

const resourcePrefix = "/passthrough"

func formatPassthroughResource(resource string) string {
	return resourcePrefix + resource
}

// NewPassthroughHandler will initialize the pools/ resources endpoint
func NewPassthroughHandler(e *echo.Echo, ptu mvc.PassthroughUsecase, ou mvc.OrderBookUsecase, logger log.Logger) {
	handler := &PassthroughHandler{
		PUsecase: ptu,
		OUsecase: ou,
		Logger:   logger,
	}

	e.GET(formatPassthroughResource("/portfolio-assets/:address"), handler.GetPortfolioAssetsByAddress)
	e.GET(formatPassthroughResource("/active-orders"), handler.GetActiveOrders)
	e.GET(formatPassthroughResource("/active-orders"), func(c echo.Context) error {
		if c.QueryParam("sse") != "" {
			return handler.GetActiveOrdersStream(c) // Server-Sent Events (SSE)
		}
		return handler.GetActiveOrders(c)
	})
}

// @Summary Returns portfolio assets associated with the given address by category.
// @Description The returned data represents the potfolio asset breakdown by category for the specified address.
// The categories include user balances, unstaking, staked, in-locks, pooled, unclaimed rewards, and total.
// The user balances and total assets are brokend down by-coin with the capitalization of the entire account value.
//
// @Produce  json
// @Success 200  {object}  passthroughdomain.PortfolioAssetsResult  "Portfolio assets by-category and capitalization of the entire account value"
// @Failure 500  {object}  domain.ResponseError  "Response error"
// @Param address path string true "Wallet Address"
// @Router /passthrough/portfolio-assets/{address} [get]
func (a *PassthroughHandler) GetPortfolioAssetsByAddress(c echo.Context) error {
	address := c.Param("address")

	if address == "" {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: "invalid address: cannot be empty"})
	}

	portfolioAssetsResult, err := a.PUsecase.GetPortfolioAssets(c.Request().Context(), address)
	if err != nil {
		return c.JSON(http.StatusPartialContent, domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, portfolioAssetsResult)
}

func (a *PassthroughHandler) GetActiveOrdersStream(c echo.Context) error {
	var (
		req types.GetActiveOrdersRequest
		err error
	)

	ctx, span := deliveryhttp.Span(c)
	defer func() {
		deliveryhttp.RecordSpanError(ctx, span, err)
	}()

	if err := deliveryhttp.ParseRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	w := c.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := a.OUsecase.GetActiveOrdersStream(ctx, req.UserOsmoAddress)

	for {
		select {
		case <-c.Request().Context().Done():
			return c.NoContent(http.StatusOK)
		case orders, ok := <-ch:
			if !ok {
				return c.NoContent(http.StatusOK)
			}

			if orders.Error != nil {
				a.Logger.Error("GET "+c.Request().URL.String(), zap.Error(orders.Error))
			}

			err := deliveryhttp.WriteEvent(
				w,
				types.NewGetAllOrderResponse(orders.LimitOrders, orders.IsBestEffort),
			)

			if err != nil {
				a.Logger.Error("GET "+c.Request().URL.String(), zap.Error(err))
			}
		}
	}
}

// @Summary Returns all active orderbook orders associated with the given address.
// @Description The returned data represents all active orders for all orderbooks available for the specified address.
//
// The is_best_effort flag indicates whether the error occurred while processing the orders due which not all orders were returned in the response.
//
// @Produce  json
// @Success 200           {object}  types.GetActiveOrdersResponse  "List of active orders for all available orderboooks for the given address"
// @Failure 400           {object}  domain.ResponseError                 "Response error"
// @Failure 500           {object}  domain.ResponseError                 "Response error"
// @Param  userOsmoAddress  query  string  true  "Osmo wallet address"
// @Router /passthrough/active-orders [get]
func (a *PassthroughHandler) GetActiveOrders(c echo.Context) error {
	var (
		req types.GetActiveOrdersRequest
		err error
	)

	ctx, span := deliveryhttp.Span(c)
	defer func() {
		deliveryhttp.RecordSpanError(ctx, span, err)
	}()

	if err = deliveryhttp.ParseRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	orders, isBestEffort, err := a.OUsecase.GetActiveOrders(ctx, req.UserOsmoAddress)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: types.ErrInternalError.Error()})
	}

	resp := types.NewGetAllOrderResponse(orders, isBestEffort)

	return c.JSON(http.StatusOK, resp)
}
