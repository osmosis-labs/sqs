package http

import (
	"net/http"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"
	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/pools"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
	sqspassthroughdomain "github.com/osmosis-labs/sqs/sqsdomain/passthroughdomain"
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
	SpreadFactor      osmomath.Dec `json:"spread_factor"`
	LiquidityCap      osmomath.Int `json:"liquidity_cap"`
	LiquidityCapError string       `json:"liquidity_cap_error"`

	APRData  sqspassthroughdomain.PoolAPRDataStatusWrap  `json:"apr_data,omitempty"`
	FeesData sqspassthroughdomain.PoolFeesDataStatusWrap `json:"fees_data,omitempty"`
}

// GetPoolsResponse is a structure for serializing pools result returned to clients.
type GetPoolsResponse struct {
	Data []PoolResponse              `json:"data"`
	Meta *v1beta1.PaginationResponse `json:"meta"`
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
	e.GET(formatPoolsResource("/canonical-orderbook"), handler.GetCanonicalOrderbook)
	e.GET(formatPoolsResource("/canonical-orderbooks"), handler.GetCanonicalOrderbooks)
	e.GET(formatPoolsResource(""), handler.GetPools)
}

// @Summary Get pool(s) information
// @Description Returns a list of pools if the IDs parameter is not given. Otherwise,
// @Description it batch fetches specific pools by the given pool IDs parameter.
// @ID get-pools
// @Produce  json
// @Param  IDs  query  string  false  "Comma-separated list of pool IDs to fetch, e.g., '1,2,3'"
// @Param  min_liquidity_cap  query  int  false  "Minimum pool liquidity cap"
// @Param  with_market_incentives  query  bool  false  "Include market incentives data in the pool response"
// @Success 200  {array}  sqsdomain.PoolI  "List of pool(s) details"
// @Router /pools [get]
func (a *PoolsHandler) GetPools(c echo.Context) error {
	var req api.GetPoolsRequest
	if err := deliveryhttp.ParseRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	var (
		pools []sqsdomain.PoolI
	)

	filters := []domain.PoolsOption{
		domain.WithFilter(req.Filter),
		domain.WithPagination(req.Pagination),
		domain.WithSort(req.Sort),
	}

	// Get pools
	pools, total, err := a.PUsecase.GetPools(
		filters...,
	)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	// Convert pools to the appropriate format
	resultPools := convertPoolsToResponse(c, &req, pools, total)

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

// @Summary Get canonical orderbook pool ID for the given base and quote.
// @Description Returns the canonical orderbook pool ID for the given base and quote.
// @Description if the pool ID is not found for the given pair, it returns an error.
// @Description if the base or quote denom are not provided, it returns an error.
// @Produce  json
// @Param  base  query  string  true  "Base denom"
// @Param  quote  query  string  true  "Quote denom"
// @Success 200  {object} domain.CanonicalOrderBooksResult  "Canonical Orderbook Pool ID for the given base and quote"
// @Router /pools/canonical-orderbook [get]
func (a *PoolsHandler) GetCanonicalOrderbook(c echo.Context) error {
	base := c.QueryParam("base")
	if base == "" {
		return c.JSON(http.StatusBadRequest, ResponseError{Message: "base must be provided"})
	}

	quote := c.QueryParam("quote")
	if quote == "" {
		return c.JSON(http.StatusBadRequest, ResponseError{Message: "quote must be provided"})
	}

	poolID, contractAddres, err := a.PUsecase.GetCanonicalOrderbookPool(base, quote)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, domain.CanonicalOrderBooksResult{
		Base:            base,
		Quote:           quote,
		PoolID:          poolID,
		ContractAddress: contractAddres,
	})
}

// @Summary Get entries for all supported orderbook base and quote denoms.
// @Description Returns the list of canonical orderbook pool ID entries for all possible base and quote combinations.
// @Produce  json
// @Success 200  {array}  domain.CanonicalOrderBooksResult  "List of canonical orderbook ool ID entries for all base and quotes"
// @Router /pools/canonical-orderbooks [get]
func (a *PoolsHandler) GetCanonicalOrderbooks(c echo.Context) error {
	orderbookData, err := a.PUsecase.GetAllCanonicalOrderbookPoolIDs()
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, orderbookData)
}

// convertPoolToResponse convertes a given pool to the appropriate response type.
func convertPoolToResponse(pool sqsdomain.PoolI) PoolResponse {
	return PoolResponse{
		ChainModel:        pool.GetUnderlyingPool(),
		Balances:          pool.GetSQSPoolModel().Balances,
		Type:              pool.GetType(),
		SpreadFactor:      pool.GetSQSPoolModel().SpreadFactor,
		LiquidityCap:      pool.GetLiquidityCap(),
		LiquidityCapError: pool.GetLiquidityCapError(),
		APRData:           pool.GetAPRData(),
		FeesData:          pool.GetFeesData(),
	}
}

// convertPoolsToResponse converts the given pools to the appropriate response type.
func convertPoolsToResponse(c echo.Context, req *api.GetPoolsRequest, p []sqsdomain.PoolI, total uint64) any {
	pools := make([]PoolResponse, 0, len(p))
	for _, pool := range p {
		pools = append(pools, convertPoolToResponse(pool))
	}

	if req.IsLegacy(c) {
		return pools // backward compatibility
	}

	return &GetPoolsResponse{
		Data: pools,
		Meta: v1beta1.NewPaginationResponse(req.Pagination, total),
	}
}
