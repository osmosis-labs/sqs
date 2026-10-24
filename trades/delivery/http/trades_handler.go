package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/trades/telemetry"
	"github.com/osmosis-labs/sqs/trades/types"
)

// TradesHandler is the HTTP handler for the trades endpoints.
type TradesHandler struct {
	repo mvc.TradesRepository
	now  func() time.Time
}

const resourcePrefix = "/trades"

// errNotIngested is returned until the first block's trades have been ingested,
// so that an empty store is not mistaken for no trading activity.
const errNotIngested = "trades have not been ingested yet"

// NewTradesHandler registers the /trades endpoints.
func NewTradesHandler(e *echo.Echo, repo mvc.TradesRepository) {
	handler := &TradesHandler{repo: repo, now: time.Now}

	e.GET(resourcePrefix+"/list", handler.GetTrades)
	e.GET(resourcePrefix+"/aggregates", handler.GetAggregates)
}

// @Summary Returns trades for a pool or an asset, newest first.
// @Description Trades are token_swapped events ingested since SQS started, plus with trades.backfill-blocks the blocks before it that the node keeps (in-memory, bounded retention).
// @Description A multi-hop swap appears as one trade per hop. Orderbook trades have source "orderbookFill".
// @Description wallet is the swap message sender, not the fee payer; for swaps routed through a contract it is the contract.
// @Description usdValueAtIngest uses the spot price when SQS ingested the block, not the price at the trade's height. Backfilled trades are valued at their time, from a leg in the USD quote asset or the prices the trades before them imply.
// @Produce  json
// @Param  poolId            query  int     false  "Pool ID. At least one of poolId and coinMinimalDenom is required."
// @Param  coinMinimalDenom  query  string  false  "Chain minimal denom traded in or out."
// @Param  wallet            query  string  false  "Bech32 address of the swap message sender."
// @Param  limit             query  int     false  "Page size, 1 to 100. Defaults to 50."
// @Param  cursor            query  string  false  "nextCursor from the previous page."
// @Success 200  {object}  types.TradesListResponse  "Trades"
// @Failure 400  {object}  domain.ResponseError  "Invalid request"
// @Failure 503  {object}  domain.ResponseError  "Trades not ingested yet, or the startup backfill is still running"
// @Router /trades/list [get]
func (h *TradesHandler) GetTrades(c echo.Context) error {
	var (
		req types.ListTradesRequest
		err error
	)

	ctx, span := deliveryhttp.Span(c)
	defer func() {
		deliveryhttp.RecordSpanError(ctx, span, err)
	}()

	if err = deliveryhttp.ParseRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	telemetry.ListRequestsCounter.WithLabelValues(req.ScopeLabel(), strconv.FormatBool(req.Wallet != nil)).Inc()

	if h.repo.LastIngestedHeight() == 0 {
		return c.JSON(http.StatusServiceUnavailable, domain.ResponseError{Message: errNotIngested})
	}

	trades, next := h.repo.ListTrades(req.Filter(), req.Cursor, req.Limit)

	return c.JSON(http.StatusOK, types.NewTradesListResponse(trades, next))
}

// @Summary Returns rolling 24h and 7d trade aggregates for a pool or an asset.
// @Description With coinMinimalDenom, a buy is a trade that received that denom and a sell one that gave it up.
// @Description With only poolId, buys and sells follow each trade's direction and trades without one are not counted.
// @Description Volumes are sums of usdValueAtIngest over priced trades and are null when no trade on that side was priced.
// @Description Windows only cover trades ingested since SQS started, plus any backfill.
// @Produce  json
// @Param  poolId            query  int     false  "Pool ID. At least one of poolId and coinMinimalDenom is required."
// @Param  coinMinimalDenom  query  string  false  "Chain minimal denom."
// @Param  wallet            query  string  false  "Bech32 address of the swap message sender."
// @Param  windows           query  string  false  "Comma-separated subset of 24h,7d. Defaults to both."
// @Success 200  {object}  types.AggregatesResponse  "Aggregates keyed by window"
// @Failure 400  {object}  domain.ResponseError  "Invalid request"
// @Failure 503  {object}  domain.ResponseError  "Trades not ingested yet, or the startup backfill is still running"
// @Router /trades/aggregates [get]
func (h *TradesHandler) GetAggregates(c echo.Context) error {
	var (
		req types.AggregatesRequest
		err error
	)

	ctx, span := deliveryhttp.Span(c)
	defer func() {
		deliveryhttp.RecordSpanError(ctx, span, err)
	}()

	if err = deliveryhttp.ParseRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	telemetry.AggregatesRequestsCounter.WithLabelValues(req.ScopeLabel(), strconv.FormatBool(req.Wallet != nil)).Inc()

	if h.repo.LastIngestedHeight() == 0 {
		return c.JSON(http.StatusServiceUnavailable, domain.ResponseError{Message: errNotIngested})
	}

	aggregates := h.repo.GetAggregates(req.Filter(), req.Windows, h.now())

	return c.JSON(http.StatusOK, types.AggregatesResponse(aggregates))
}
