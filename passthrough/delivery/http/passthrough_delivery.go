package http

import (
	"net/http"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

// ResponseError represent the response error struct
type ResponseError struct {
	Message string `json:"message"`
}

// PoolsHandler  represent the httphandler for pools
type PassthroughHandler struct {
	PUsecase mvc.PassthroughUsecase
}

// PoolsResponse is a structure for serializing pool result returned to clients.
type UserBalanceResponse struct {
	Balances sdk.Coins `json:"balances"`
}

const resourcePrefix = "/passthrough"

func formatPassthroughResource(resource string) string {
	return resourcePrefix + resource
}

func NewPassthroughHandler(e *echo.Echo, us mvc.PassthroughUsecase) {
	handler := &PassthroughHandler{
		PUsecase: us,
	}

	e.GET(formatPassthroughResource("/balances/:address"), handler.GetAccountBalances)
}

// @Summary Get account balances
// @Description Returns the balances of the account associated with the provided address.
// @ID get-account-balances
// @Produce  json
// @Param  address  param  string  false  "User address"
// @Success 200  {array}  sdk.Coins  "List of coins"
// @Router /balances/address [get]
func (a *PassthroughHandler) GetAccountBalances(c echo.Context) error {
	// Get user address as string.
	address := c.Param("address")

	ctx := c.Request().Context()

	balances, err := a.PUsecase.GetBalances(ctx, address)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, balances)
}
