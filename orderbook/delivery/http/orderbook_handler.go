package http

import (
	"net/http"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/orderbook/types"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
)

// ResponseError represent the response error struct
// TODO: move to common package
type ResponseError struct {
	Message string `json:"message"`
}

// OrderbookHandler  represent the httphandler for pools
type OrderbookHandler struct {
	OUsecase mvc.OrderBookUsecase
}

const resourcePrefix = "/orderbook"

func formatOrderbookResource(resource string) string {
	return resourcePrefix + resource
}

// NewOrderbookHandler will initialize the /orderbook resources endpoint
func NewOrderbookHandler(e *echo.Echo, us mvc.OrderBookUsecase, logger log.Logger) {
	handler := &OrderbookHandler{
		OUsecase: us,
	}

	e.GET(formatOrderbookResource("/active-orders"), handler.GetActiveOrders)
}

func (a *OrderbookHandler) GetActiveOrders(c echo.Context) (err error) {
	ctx := c.Request().Context()

	span := trace.SpanFromContext(ctx)
	defer func() {
		if err != nil {
			span.RecordError(err)
			// nolint:errcheck // ignore error
			c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
		}

		// Note: we do not end the span here as it is ended in the middleware.
	}()

	var req types.GetActiveOrdersRequest
	if err := deliveryhttp.UnmarshalRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	// Validate the request
	if err := req.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	orders, err := a.OUsecase.GetActiveOrders(ctx, req.UserOsmoAddress)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: err.Error()})
	}

	resp := types.NewGetAllOrderResponse(orders)

	return c.JSON(http.StatusOK, resp)
}
