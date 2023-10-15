package http

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo"
	"github.com/osmosis-labs/router/domain"
	"github.com/sirupsen/logrus"
)

// ResponseError represent the response error struct
type ResponseError struct {
	Message string `json:"message"`
}

// QuoteHandler  represent the httphandler for article
type QuoteHandler struct {
	QUsecase domain.QuoteUsecase
}

// NewQuoteHandler will initialize the quote/ resources endpoint
func NewQuoteHandler(e *echo.Echo, us domain.QuoteUsecase) {
	handler := &QuoteHandler{
		QUsecase: us,
	}
	e.GET("/get_out_by_in", handler.GetOutByIn)
}

// GetOutByIn will fetch the quote out by token in based on given params
func (a *QuoteHandler) GetOutByIn(c echo.Context) error {
	poolIDStr := c.QueryParam("poolID")

	poolID, err := strconv.Atoi(poolIDStr)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	tokenIn := c.QueryParam("tokenIn")
	tokenOutDenom := c.QueryParam("tokenOutDenom")
	// TODO: do we need this?
	spreadFactor := c.QueryParam("spreadFactor")
	ctx := c.Request().Context()

	amount, err := a.QUsecase.GetOutByTokenIn(ctx, uint64(poolID), tokenIn, tokenOutDenom, spreadFactor)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	// c.Response().Header().Set(`X-Cursor`, nextCursor)
	return c.JSON(http.StatusOK, amount)
}

func getStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}

	logrus.Error(err)
	switch err {
	case domain.ErrInternalServerError:
		return http.StatusInternalServerError
	case domain.ErrNotFound:
		return http.StatusNotFound
	case domain.ErrConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
