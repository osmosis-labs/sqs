package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
)

// RouterHandler  represent the httphandler for the router
type RouterHandler struct {
	RUsecase mvc.RouterUsecase
	TUsecase mvc.TokensUsecase
	logger   log.Logger
}

const routerResource = "/router"

var (
	oneDec = osmomath.OneDec()
)

func formatRouterResource(resource string) string {
	return routerResource + resource
}

// NewRouterHandler will initialize the pools/ resources endpoint
func NewRouterHandler(e *echo.Echo, us mvc.RouterUsecase, tu mvc.TokensUsecase, logger log.Logger) {
	handler := &RouterHandler{
		RUsecase: us,
		TUsecase: tu,
		logger:   logger,
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
// @Description returns the best quote it can compute for the given tokenIn and tokenOutDenom.
// If `singleRoute` parameter is set to true, it gives the best single quote while excluding splits.
// @ID get-route-quote
// @Produce  json
// @Param  tokenIn  query  string  true  "String representation of the sdk.Coin for the token in."
// @Param  tokenOutDenom  query  string  true  "String representing the denom of the token out."
// @Param  singleRoute  query  bool  false  "Boolean flag indicating whether to return single routes (no splits). False (splits enabled) by default."
// @Param humanDenoms query bool true "Boolean flag indicating whether the given denoms are human readable or not. Human denoms get converted to chain internally"
// @Param  applyExponents  query  bool  false  "Boolean flag indicating whether to apply exponents to the spot price. False by default."
// @Success 200  {object}  domain.Quote  "The computed best route quote"
// @Router /router/quote [get]
func (a *RouterHandler) GetOptimalQuote(c echo.Context) (err error) {
	ctx := c.Request().Context()

	isSingleRouteStr := c.QueryParam("singleRoute")
	isSingleRoute := false
	if isSingleRouteStr != "" {
		isSingleRoute, err = strconv.ParseBool(isSingleRouteStr)
		if err != nil {
			return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
		}
	}

	shouldApplyExponentsStr := c.QueryParam("applyExponents")
	shouldApplyExponents := false
	if shouldApplyExponentsStr != "" {
		shouldApplyExponents, err = strconv.ParseBool(shouldApplyExponentsStr)
		if err != nil {
			return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
		}
	}

	tokenOutDenom, tokenIn, err := getValidRoutingParameters(c)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	chainDenoms, err := mvc.ValidateChainDenomsQueryParam(c, a.TUsecase, []string{tokenIn.Denom, tokenOutDenom})
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	// Update coins token in denom it case it was translated from human to chain.
	tokenIn.Denom = chainDenoms[0]
	tokenOutDenom = chainDenoms[1]

	var quote domain.Quote
	if isSingleRoute {
		quote, err = a.RUsecase.GetBestSingleRouteQuote(ctx, tokenIn, tokenOutDenom)
	} else {
		quote, err = a.RUsecase.GetOptimalQuote(ctx, tokenIn, tokenOutDenom)
	}
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	scalingFactor := oneDec
	if shouldApplyExponents {
		scalingFactor = a.getSpotPriceScalingFactor(tokenIn.Denom, tokenOutDenom)
	}

	_, _, err = quote.PrepareResult(ctx, scalingFactor)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, quote)
}

// GetDirectCustomQuote returns a direct custom quote. It does not search for the route.
// It directly computes the quote for the given poolID.
func (a *RouterHandler) GetDirectCustomQuote(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidRoutingParameters(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	poolIDStr := c.QueryParam("poolID")
	if len(poolIDStr) == 0 {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: "poolID is required"})
	}

	shouldApplyExponentsStr := c.QueryParam("applyExponents")
	shouldApplyExponents := false
	if shouldApplyExponentsStr != "" {
		shouldApplyExponents, err = strconv.ParseBool(shouldApplyExponentsStr)
		if err != nil {
			return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
		}
	}

	poolID, err := strconv.ParseUint(poolIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	// Quote
	quote, err := a.RUsecase.GetCustomDirectQuote(ctx, tokenIn, tokenOutDenom, poolID)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	scalingFactor := oneDec
	if shouldApplyExponents {
		scalingFactor = a.getSpotPriceScalingFactor(tokenIn.Denom, tokenOutDenom)
	}

	_, _, err = quote.PrepareResult(ctx, scalingFactor)
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

// getValidRoutingParameters returns the tokenIn and tokenOutDenom from server context if they are valid.
func getValidRoutingParameters(c echo.Context) (string, sdk.Coin, error) {
	tokenOutStr, tokenInStr, err := getValidTokenInTokenOutStr(c)
	if err != nil {
		return "", sdk.Coin{}, err
	}

	tokenIn, err := sdk.ParseCoinNormalized(tokenInStr)
	if err != nil {
		return "", sdk.Coin{}, errors.New("tokenIn is invalid - must be in the format amountDenom")
	}

	return tokenOutStr, tokenIn, nil
}

// getSpotPriceScalingFactor returns the spot price scaling factor for a given tokenIn and tokenOutDenom.
func (a *RouterHandler) getSpotPriceScalingFactor(tokenInDenom, tokenOutDenom string) osmomath.Dec {
	scalingFactor, err := a.TUsecase.GetSpotPriceScalingFactorByDenom(tokenOutDenom, tokenInDenom)
	if err != nil {
		// Note that we do not fail the quote if scaling factor fetching fails.
		// Instead, we simply set it to zero to validate future calculations downstream.
		scalingFactor = sdk.ZeroDec()
	}

	return scalingFactor
}

func getValidTokenInStr(c echo.Context) (string, error) {
	tokenInStr := c.QueryParam("tokenIn")

	if len(tokenInStr) == 0 {
		return "", errors.New("tokenIn is required")
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
		return "", "", errors.New("tokenOutDenom is required")
	}

	return tokenOutStr, tokenInStr, nil
}
