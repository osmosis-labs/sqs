package http

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/types"
)

// RouterHandler  represent the httphandler for the router
type RouterHandler struct {
	RUsecase       mvc.RouterUsecase
	TUsecase       mvc.TokensUsecase
	QuoteSimulator domain.QuoteSimulator
	logger         log.Logger
}

const routerResource = "/router"

var (
	oneDec = osmomath.OneDec()
)

func formatRouterResource(resource string) string {
	return routerResource + resource
}

// NewRouterHandler will initialize the pools/ resources endpoint
func NewRouterHandler(e *echo.Echo, us mvc.RouterUsecase, tu mvc.TokensUsecase, qs domain.QuoteSimulator, logger log.Logger) {
	handler := &RouterHandler{
		RUsecase:       us,
		TUsecase:       tu,
		QuoteSimulator: qs,
		logger:         logger,
	}
	e.GET(formatRouterResource("/quote"), handler.GetOptimalQuote)
	e.GET(formatRouterResource("/routes"), handler.GetCandidateRoutes)
	e.GET(formatRouterResource("/cached-routes"), handler.GetCachedCandidateRoutes)
	e.GET(formatRouterResource("/spot-price-pool/:id"), handler.GetSpotPriceForPool)
	e.GET(formatRouterResource("/custom-direct-quote"), handler.GetDirectCustomQuote)
	e.GET(formatRouterResource("/taker-fee-pool/:id"), handler.GetTakerFee)
	e.POST(formatRouterResource("/store-state"), handler.StoreRouterStateInFiles)
	e.GET(formatRouterResource("/state"), handler.GetRouterState)
}

// @Summary Optimal Quote
// @Description Returns the best quote it can compute for the exact in or exact out token swap method.
// @Description
// @Description For exact amount in swap method, the `tokenIn` and `tokenOutDenom` are required.
// @Description For exact amount out swap method, the `tokenOut` and `tokenInDenom` are required.
// @Description Mixing swap method parameters in other way than specified will result in an error.
// @Description
// @Description When `singleRoute` parameter is set to true, it gives the best single quote while excluding splits.
// @ID get-route-quote
// @Produce  json
// @Param  tokenIn         query  string  false  "String representation of the sdk.Coin denoting the input token for the exact amount in swap method."     example(1000000uosmo)
// @Param  tokenOutDenom   query  string  false  "String representing the denomination of the output token for the exact amount in swap method."           example(uion)
// @Param  tokenOut        query  string  false  "String representation of the sdk.Coin denoting the output token for the exact amount out swap method."   example(2353uion)
// @Param  tokenInDenom    query  string  false  "String representing the denomination of the input token for the exact amount out swap method."           example(uosmo)
// @Param  singleRoute     query  bool    false  "Boolean flag indicating whether to return single routes (no splits). False (splits enabled) by default."
// @Param  humanDenoms     query  bool    true "Boolean flag indicating whether the given denoms are human readable or not. Human denoms get converted to chain internally"
// @Param  applyExponents  query  bool    false  "Boolean flag indicating whether to apply exponents to the spot price. False by default."
// @Param  simulatorAddress query string false "Address of the simulator to simulate the quote. If provided, the quote will be simulated."
// @Param  simulationSlippageTolerance query string false "Slippage tolerance multiplier for the simulation. If simulatorAddress is provided, this must be provided."
// @Param  appendBaseFee query bool false "Boolean flag indicating whether to append the base fee to the quote. False by default."
// @Success 200  {object}  domain.Quote  "The computed best route quote"
// @Router /router/quote [get]
func (a *RouterHandler) GetOptimalQuote(c echo.Context) (err error) {
	ctx := c.Request().Context()

	span := trace.SpanFromContext(ctx)
	defer func() {
		if r := recover(); r != nil {
			// nolint:errcheck // ignore error
			c.JSON(http.StatusInternalServerError, domain.ResponseError{Message: fmt.Sprintf("panic: %v", r)})
		}

		if err != nil {
			span.RecordError(err)
			// nolint:errcheck // ignore error
			c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
		}

		// Note: we do not end the span here as it is ended in the middleware.
	}()

	var req types.GetQuoteRequest
	if err := deliveryhttp.ParseRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	var (
		tokenIn       *sdk.Coin
		tokenOutDenom string
	)

	if req.SwapMethod() == domain.TokenSwapMethodExactIn {
		tokenIn, tokenOutDenom = req.TokenIn, req.TokenOutDenom
	} else {
		tokenIn, tokenOutDenom = req.TokenOut, req.TokenInDenom
	}

	chainDenoms, err := mvc.ValidateChainDenomsQueryParam(c, a.TUsecase, []string{tokenIn.Denom, tokenOutDenom})
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	// Update coins token in denom it case it was translated from human to chain.
	tokenIn.Denom = chainDenoms[0]
	tokenOutDenom = chainDenoms[1]

	var routerOpts []domain.RouterOption
	if req.SingleRoute {
		routerOpts = append(routerOpts, domain.WithMaxSplitRoutes(domain.DisableSplitRoutes))
	}

	var quote domain.Quote
	if req.SwapMethod() == domain.TokenSwapMethodExactIn {
		quote, err = a.RUsecase.GetOptimalQuote(ctx, *tokenIn, tokenOutDenom, routerOpts...)
	} else {
		quote, err = a.RUsecase.GetOptimalQuoteInGivenOut(ctx, *tokenIn, tokenOutDenom, routerOpts...)
	}

	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	scalingFactor := oneDec
	if req.ApplyExponents {
		scalingFactor = a.getSpotPriceScalingFactor(tokenIn.Denom, tokenOutDenom)
	}

	_, _, err = quote.PrepareResult(ctx, scalingFactor, a.logger)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	span.SetAttributes(attribute.Stringer("token_out", quote.GetAmountOut()))
	span.SetAttributes(attribute.Stringer("price_impact", quote.GetPriceImpact()))

	// Simulate quote if applicable.
	// Note: only single routes (non-splits) are supported for simulation.
	// Additionally, the functionality is triggerred by the user providing a simulator address.
	// Only "out given in" swap method is supported for simulation. Thus, we also check for tokenOutDenom being set.
	simulatorAddress := req.SimulatorAddress
	if req.SingleRoute && simulatorAddress != "" && req.SwapMethod() == domain.TokenSwapMethodExactIn {
		priceInfo := a.QuoteSimulator.SimulateQuote(ctx, quote, req.SlippageToleranceMultiplier, simulatorAddress)

		// Set the quote price info.
		quote.SetQuotePriceInfo(&priceInfo)
	}

	if req.AppendBaseFee {
		quote.SetQuotePriceInfo(&domain.TxFeeInfo{
			BaseFee: a.RUsecase.GetBaseFee().CurrentFee,
		})
	}

	return c.JSON(http.StatusOK, quote)
}

// @Summary Compute the quote for the given poolID
// @Description Call does not search for the route rather directly computes the quote for the given poolID.
// @Description NOTE: Endpoint only supports multi-hop routes, split routes are not supported.
// @Description
// @Description For exact amount in swap method, the `tokenIn` and `tokenOutDenom` are required.
// @Description For exact amount out swap method, the `tokenOut` and `tokenInDenom` are required.
// @Description Mixing swap method parameters in other way than specified will result in an error.
// @Description
// @ID get-direct-quote
// @Produce  json
// @Param  tokenIn         query  string  false  "String representation of the sdk.Coin denoting the input token for the exact amount in swap method."                       example(1000000uosmo)
// @Param  tokenOutDenom   query  string  false  "String representing the list of the output token denominations separated by comma for the exact amount in swap method."    example(uion)
// @Param  tokenOut        query  string  false  "String representation of the sdk.Coin denoting the output token for the exact amount out swap method."                     example(2353uion)
// @Param  tokenInDenom    query  string  false  "String representing the list of the input token denominations separated by comma for the exact amount out swap method."    example(uosmo)
// @Param  poolID          query  string  true   "String representing list of the pool ID."                                                                                  example(1100)
// @Param  humanDenoms     query  bool    true   "Boolean flag indicating whether the given denoms are human readable or not. Human denoms get converted to chain internally"
// @Param  applyExponents  query  bool    false  "Boolean flag indicating whether to apply exponents to the spot price. False by default."
// @Success 200  {object}  domain.Quote  "The computed best route quote"
// @Router /router/custom-direct-quote [get]
func (a *RouterHandler) GetDirectCustomQuote(c echo.Context) (err error) {
	ctx := c.Request().Context()

	defer func() {
		if err != nil {
			// nolint:errcheck // ignore error
			c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
		}

		// Note: we do not end the span here as it is ended in the middleware.
	}()

	var req types.GetDirectCustomQuoteRequest
	if err := deliveryhttp.ParseRequest(c, &req); err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	var (
		tokenIn       *sdk.Coin
		tokenOutDenom []string
	)

	// Determine the tokenIn and tokenOutDenom based on the swap method.
	if req.SwapMethod() == domain.TokenSwapMethodExactIn {
		tokenIn, tokenOutDenom = req.TokenIn, req.TokenOutDenom
	} else {
		tokenIn, tokenOutDenom = req.TokenOut, req.TokenInDenom
	}

	// Apply human denoms conversion if required.
	chainDenoms, err := mvc.ValidateChainDenomsQueryParam(c, a.TUsecase, append([]string{tokenIn.Denom}, tokenOutDenom...))
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	// Update coins token in denom it case it was translated from human to chain.
	tokenIn.Denom = chainDenoms[0]
	tokenOutDenom = chainDenoms[1:]

	// Get the quote based on the swap method.
	var quote domain.Quote
	if req.SwapMethod() == domain.TokenSwapMethodExactIn {
		quote, err = a.RUsecase.GetCustomDirectQuoteMultiPool(ctx, *tokenIn, tokenOutDenom, req.PoolID)
	} else {
		quote, err = a.RUsecase.GetCustomDirectQuoteMultiPoolInGivenOut(ctx, *tokenIn, tokenOutDenom, req.PoolID)
	}
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	scalingFactor := oneDec
	if req.ApplyExponents {
		scalingFactor = a.getSpotPriceScalingFactor(tokenIn.Denom, tokenOutDenom[len(tokenOutDenom)-1])
	}

	_, _, err = quote.PrepareResult(ctx, scalingFactor, a.logger)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, quote)
}

// @Summary Token Routing Information
// @Description returns all routes that can be used for routing from tokenIn to tokenOutDenom.
// @ID get-router-routes
// @Produce  json
// @Param  tokenIn  query  string  true  "The string representation of the denom of the token in"
// @Param  tokenOutDenom  query  string  true  "The string representation of the denom of the token out"
// @Param humanDenoms query bool true "Boolean flag indicating whether the given denoms are human readable or not. Human denoms get converted to chain internally"
// @Success 200  {array}  sqsdomain.CandidateRoutes  "An array of possible routing options"
// @Router /router/routes [get]
func (a *RouterHandler) GetCandidateRoutes(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidTokenInTokenOutStr(c)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	chainDenoms, err := mvc.ValidateChainDenomsQueryParam(c, a.TUsecase, []string{tokenIn, tokenOutDenom})
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	// Update the tokenIn and tokenOutDenom with the chain denoms if they were translated from human to chain.
	tokenIn = chainDenoms[0]
	tokenOutDenom = chainDenoms[1]

	routes, err := a.RUsecase.GetCandidateRoutes(ctx, sdk.NewCoin(tokenIn, osmomath.OneInt()), tokenOutDenom)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	if err := c.JSON(http.StatusOK, routes); err != nil {
		return err
	}
	return nil
}

func (a *RouterHandler) GetTakerFee(c echo.Context) error {
	idStr := c.Param("id")
	poolID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	takerFees, err := a.RUsecase.GetTakerFee(poolID)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, takerFees)
}

// GetCandidateRoutes returns the candidate routes for a given tokenIn and tokenOutDenom from cache.
// If no routes present in cache, it does not attempt to recompute them.
func (a *RouterHandler) GetCachedCandidateRoutes(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidTokenInTokenOutStr(c)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	routes, _, err := a.RUsecase.GetCachedCandidateRoutes(ctx, tokenIn, tokenOutDenom)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, routes)
}

// TODO: authentication for the endpoint and enable only in dev mode.
func (a *RouterHandler) StoreRouterStateInFiles(c echo.Context) error {
	if err := a.RUsecase.StoreRouterStateFiles(); err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, "Router state stored in files")
}

func (a *RouterHandler) GetRouterState(c echo.Context) error {
	routerState, err := a.RUsecase.GetRouterState()
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, routerState)
}

// GetSpotPrice returns the spot price for a given poolID, quoteAsset and baseAsset
func (a *RouterHandler) GetSpotPriceForPool(c echo.Context) error {
	ctx := c.Request().Context()

	idStr := c.Param("id")
	poolID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	quoteAsset := c.QueryParam("quoteAsset")
	if len(quoteAsset) == 0 {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: "quoteAsset is required"})
	}
	baseAsset := c.QueryParam("baseAsset")
	if len(baseAsset) == 0 {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: "baseAsset is required"})
	}

	spotPrice, err := a.RUsecase.GetPoolSpotPrice(ctx, poolID, quoteAsset, baseAsset)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, spotPrice)
}

// getSpotPriceScalingFactor returns the spot price scaling factor for a given tokenIn and tokenOutDenom.
func (a *RouterHandler) getSpotPriceScalingFactor(tokenInDenom, tokenOutDenom string) osmomath.Dec {
	scalingFactor, err := a.TUsecase.GetSpotPriceScalingFactorByDenom(tokenOutDenom, tokenInDenom)
	if err != nil {
		// Note that we do not fail the quote if scaling factor fetching fails.
		// Instead, we simply set it to zero to validate future calculations downstream.
		scalingFactor = osmomath.ZeroDec()
	}

	return scalingFactor
}

func getValidTokenInStr(c echo.Context) (string, error) {
	tokenInStr := c.QueryParam("tokenIn")

	if len(tokenInStr) == 0 {
		return "", types.ErrTokenInNotSpecified
	}

	return tokenInStr, nil
}

func getValidTokenInTokenOutStr(c echo.Context) (tokenOutStr, tokenInStr string, err error) {
	tokenInStr, err = getValidTokenInStr(c)
	if err != nil {
		return "", "", err
	}

	tokenOutStr = c.QueryParam("tokenOutDenom")

	if len(tokenOutStr) == 0 {
		return "", "", types.ErrTokenOutDenomNotSpecified
	}

	// Validate input denoms
	if err := domain.ValidateInputDenoms(tokenInStr, tokenOutStr); err != nil {
		return "", "", err
	}

	return tokenOutStr, tokenInStr, nil
}
