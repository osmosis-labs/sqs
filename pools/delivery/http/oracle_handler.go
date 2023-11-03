package http

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo"
	"github.com/osmosis-labs/sqs/domain"
)

type OracleHandler struct {
	usecase domain.OracleUsecase
}

func NewOracleHandler(e *echo.Echo, us domain.OracleUsecase) {
	handler := &OracleHandler{
		usecase: us,
	}
	// Update prices endpoint (TODO: This shouldn't be a get, but whatever)
	e.GET("/update-prices", handler.UpdatePrices)
}

// Why so much indirection here?

func (a *OracleHandler) UpdatePrices(c echo.Context) error {
	ctx := c.Request().Context()

	fmt.Println("update prices")

	err := a.usecase.UpdatePrices(ctx)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, nil)
}
