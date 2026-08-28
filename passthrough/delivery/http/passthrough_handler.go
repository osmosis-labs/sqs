package http

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/osmosis-labs/osmosis/osmomath"
	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	_ "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/orderbook/types"

	"github.com/labstack/echo/v4"

	"go.uber.org/zap"
)

// subtractUnrealizedCancels subtracts unrealized cancels from total_amount_of_liquidity.
// Both inputs use the same unit (minimal token amount as an integer string / osmomath.Int).
//
// An empty tal is treated as zero liquidity: the ingested tick state leaves the field
// empty for a side that has never held liquidity.
//
// Two conditions are treated as errors rather than being smoothed over, because both
// mean the ingested state is wrong and publishing a plausible number would hide it:
// a non-empty tal that fails to parse, and cancels exceeding the liquidity they are
// cancelling (which violates the orderbook invariant that a tick cannot have more
// unrealized cancels than total liquidity).
func subtractUnrealizedCancels(tal string, cancels osmomath.Int) (string, error) {
	if tal == "" {
		return "0", nil
	}
	talDec, err := osmomath.NewDecFromStr(tal)
	if err != nil {
		return "", fmt.Errorf("parsing total_amount_of_liquidity %q: %w", tal, err)
	}
	if cancels.IsNil() || !cancels.IsPositive() {
		// TotalAmountOfLiquidity is always an integer string — truncate the Dec
		return talDec.TruncateInt().String(), nil
	}
	result := talDec.Sub(osmomath.NewDecFromInt(cancels))
	if result.IsNegative() {
		return "", fmt.Errorf(
			"unrealized cancels %s exceed total_amount_of_liquidity %s",
			cancels, tal,
		)
	}
	return result.TruncateInt().String(), nil
}

// PassthroughHandler is the http handler for passthrough use case
type PassthroughHandler struct {
	PUsecase     mvc.PassthroughUsecase
	OUsecase     mvc.OrderBookUsecase
	PoolsUsecase mvc.PoolsUsecase
	Logger       log.Logger
}

const resourcePrefix = "/passthrough"

func formatPassthroughResource(resource string) string {
	return resourcePrefix + resource
}

// NewPassthroughHandler will initialize the pools/ resources endpoint
func NewPassthroughHandler(e *echo.Echo, ptu mvc.PassthroughUsecase, ou mvc.OrderBookUsecase, pu mvc.PoolsUsecase, logger log.Logger) {
	handler := &PassthroughHandler{
		PUsecase:     ptu,
		OUsecase:     ou,
		PoolsUsecase: pu,
		Logger:       logger,
	}

	e.GET(formatPassthroughResource("/portfolio-assets/:address"), handler.GetPortfolioAssetsByAddress)
	e.GET(formatPassthroughResource("/orderbook-ticks"), handler.GetOrderbookTicks)
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

// @Summary Returns all tick states for a given orderbook pool.
// @Description Returns the full tick map for the specified pool as indexed by SQS. Each tick includes
// @Description ask and bid liquidity values, with unrealized cancels already subtracted, and ticks with
// @Description no liquidity on either side omitted. Ticks are sorted by ascending tick ID.
// @Description
// @Description An empty ticks array means the pool is genuinely empty, not that data is missing:
// @Description an unknown pool or one that is not an orderbook returns 404, and an orderbook whose
// @Description ticks have not been ingested yet returns 503. Clients must not treat those as zero depth.
//
// @Produce  json
// @Success 200  {object}  types.GetOrderbookTicksResponse  "Tick states for the given pool"
// @Failure 400  {object}  domain.ResponseError             "Response error"
// @Failure 404  {object}  domain.ResponseError             "Pool is unknown or is not an orderbook"
// @Failure 500  {object}  domain.ResponseError             "Malformed or invariant-violating tick state"
// @Failure 503  {object}  domain.ResponseError             "Pool ticks have not been ingested yet"
// @Param   poolID  query  string  true  "Pool ID"
// @Router /passthrough/orderbook-ticks [get]
func (a *PassthroughHandler) GetOrderbookTicks(c echo.Context) error {
	var (
		req types.GetOrderbookTicksRequest
		err error
	)

	ctx, span := deliveryhttp.Span(c)
	defer func() {
		deliveryhttp.RecordSpanError(ctx, span, err)
	}()

	if err = deliveryhttp.ParseRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	// Distinguish "not an orderbook" from "not ingested yet" before reading the tick
	// map, so that a missing snapshot is never reported to clients as an empty book.
	//
	// This deliberately does not use IsCanonicalOrderbookPool: "canonical" means the
	// highest-liquidity orderbook for a base/quote pair, and a pool is dropped from
	// that set when a rival for the same pair overtakes it. Gating on it would 404 a
	// perfectly valid orderbook, and would make a pool's depth disappear when
	// liquidity shifted. Any orderbook pool has tick state worth serving.
	// Kept out of err: an unknown pool ID is an ordinary client mistake, not a
	// server fault, and should not be recorded as a span error.
	pool, poolErr := a.PoolsUsecase.GetPool(req.PoolID)
	if poolErr != nil {
		return c.JSON(http.StatusNotFound, domain.ResponseError{
			Message: fmt.Sprintf("pool %d not found", req.PoolID),
		})
	}

	cosmWasmPoolModel := pool.GetSQSPoolModel().CosmWasmPoolModel
	if cosmWasmPoolModel == nil || !cosmWasmPoolModel.IsOrderbook() {
		return c.JSON(http.StatusNotFound, domain.ResponseError{
			Message: fmt.Sprintf("pool %d is not an orderbook", req.PoolID),
		})
	}

	tickMap, found := a.OUsecase.GetAllTicks(req.PoolID)
	if !found {
		return c.JSON(http.StatusServiceUnavailable, domain.ResponseError{
			Message: fmt.Sprintf("ticks for pool %d have not been ingested yet", req.PoolID),
		})
	}

	ticks := make([]orderbookdomain.Tick, 0, len(tickMap))
	for tickID, tick := range tickMap {
		var askTAL, bidTAL string
		if askTAL, err = subtractUnrealizedCancels(
			tick.TickState.AskValues.TotalAmountOfLiquidity,
			tick.UnrealizedCancels.AskUnrealizedCancels,
		); err != nil {
			err = fmt.Errorf("ask liquidity for tick %d in pool %d: %w", tickID, req.PoolID, err)
			a.Logger.Error("GET "+c.Request().URL.String(), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: types.ErrInternalError.Error()})
		}
		if bidTAL, err = subtractUnrealizedCancels(
			tick.TickState.BidValues.TotalAmountOfLiquidity,
			tick.UnrealizedCancels.BidUnrealizedCancels,
		); err != nil {
			err = fmt.Errorf("bid liquidity for tick %d in pool %d: %w", tickID, req.PoolID, err)
			a.Logger.Error("GET "+c.Request().URL.String(), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: types.ErrInternalError.Error()})
		}

		if askTAL == "0" && bidTAL == "0" {
			continue
		}

		tickState := tick.TickState
		tickState.AskValues.TotalAmountOfLiquidity = askTAL
		tickState.BidValues.TotalAmountOfLiquidity = bidTAL

		ticks = append(ticks, orderbookdomain.Tick{
			TickID:    tickID,
			TickState: tickState,
		})
	}

	// Map iteration order is randomised, so sort by tick ID to keep the response
	// stable across calls. Tick ID is ascending price, which is also the order the
	// frontend renders depth in.
	sort.Slice(ticks, func(i, j int) bool { return ticks[i].TickID < ticks[j].TickID })

	return c.JSON(http.StatusOK, types.GetOrderbookTicksResponse{Ticks: ticks})
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
