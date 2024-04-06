package http

import (
	"net/http"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v23/x/poolmanager/types"
)

// ResponseError represent the response error struct
type ResponseError struct {
	Message string `json:"message"`
}

// PoolsHandler  represent the httphandler for pools
type PoolsHandler struct {
	PUsecase mvc.PoolsUsecase
}

// PoolsResponse is a structure for serializing pool result returned to clients.
type PoolResponse struct {
	ChainModel poolmanagertypes.PoolI    `json:"chain_model"`
	Balances   sdk.Coins                 `json:"balances"`
	Type       poolmanagertypes.PoolType `json:"type"`
	// In some cases, spread factor might be duplicated in the chain model.
	// However, we duplicate it here for client convinience to be able to always
	// rely on it being present.
	SpreadFactor osmomath.Dec `json:"spread_factor"`

	TotalLiquidity      sdk.Coin `json:"total_liquidity"`
	TotalLiquidityError string   `json:"total_liquidity_error"`
}

const resourcePrefix = "/pools"

func formatPoolsResource(resource string) string {
	return resourcePrefix + resource
}

// NewPoolsHandler will initialize the pools/ resources endpoint
func NewPoolsHandler(e *echo.Echo, us mvc.PoolsUsecase) {
	handler := &PoolsHandler{
		PUsecase: us,
	}

	e.GET(formatPoolsResource("/ticks/:id"), handler.GetConcentratedPoolTicks)
	e.GET(formatPoolsResource(""), handler.GetPools)
}

// @Summary Get pool(s) information
// @Description Returns a list of pools if the IDs parameter is not given. Otherwise,
// @Description it batch fetches specific pools by the given pool IDs parameter.
// @ID get-pools
// @Produce  json
// @Param  IDs  query  string  false  "Comma-separated list of pool IDs to fetch, e.g., '1,2,3'"
// @Success 200  {array}  sqsdomain.PoolI  "List of pool(s) details"
// @Router /pools [get]
func (a *PoolsHandler) GetPools(c echo.Context) error {
	// Get pool ID parameters as strings.
	poolIDsStr := c.QueryParam("IDs")

	var (
		pools []sqsdomain.PoolI
		err   error
	)

	// if IDs are not given, get all pools
	if len(poolIDsStr) == 0 {
		pools, err = a.PUsecase.GetAllPools()
		if err != nil {
			return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
		}
	} else {
		// Parse them to numbers
		poolIDs, err := domain.ParseNumbers(poolIDsStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ResponseError{Message: err.Error()})
		}

		// Get pools
		pools, err = a.PUsecase.GetPools(poolIDs)
		if err != nil {
			return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
		}
	}

	// Convert pools to the appropriate format
	resultPools := convertPoolsToResponse(pools)

	return c.JSON(http.StatusOK, resultPools)
}

func (a *PoolsHandler) GetConcentratedPoolTicks(c echo.Context) error {
	idStr := c.Param("id")
	poolID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ResponseError{Message: err.Error()})
	}

	pools, err := a.PUsecase.GetTickModelMap([]uint64{poolID})
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	tickModel, ok := pools[poolID]
	if !ok {
		return c.JSON(http.StatusNotFound, ResponseError{Message: "tick model not found for given pool"})
	}

	return c.JSON(http.StatusOK, tickModel)
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

// convertPoolToResponse convertes a given pool to the appropriate response type.
func convertPoolToResponse(pool sqsdomain.PoolI) PoolResponse {
	return PoolResponse{
		ChainModel:          pool.GetUnderlyingPool(),
		Balances:            pool.GetSQSPoolModel().Balances,
		Type:                pool.GetType(),
		SpreadFactor:        pool.GetSQSPoolModel().SpreadFactor,
		TotalLiquidity:      sdk.NewCoin("usdc", pool.GetTotalValueLockedUSDC()),
		TotalLiquidityError: pool.GetSQSPoolModel().TotalValueLockedError,
	}
}

// convertPoolsToResponse converts the given pools to the appropriate response type.
func convertPoolsToResponse(pools []sqsdomain.PoolI) []PoolResponse {
	resultPools := make([]PoolResponse, 0, len(pools))
	for _, pool := range pools {
		resultPools = append(resultPools, convertPoolToResponse(pool))
	}
	return resultPools
}
