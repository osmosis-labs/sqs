package http

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

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

// Handler Errors
var (
	ErrTokenNotValid                   = errors.New("tokenIn is invalid - must be in the format amountDenom")
	ErrTokenNotSpecified               = errors.New("tokenIn is required")
	ErrNumOfTokenOutDenomPoolsMismatch = errors.New("number of tokenOutDenom must be equal to number of pool IDs")
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

	span := trace.SpanFromContext(ctx)
	defer func() {
		if err != nil {
			span.RecordError(err)
			// nolint:errcheck // ignore error
			c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
		}

		// Note: we do not end the span here as it is ended in the middleware.
	}()

	isSingleRouteStr := c.QueryParam("singleRoute")
	isSingleRoute := false
	if isSingleRouteStr != "" {
		isSingleRoute, err = strconv.ParseBool(isSingleRouteStr)
		if err != nil {
			return err
		}
	}

	shouldApplyExponentsStr := c.QueryParam("applyExponents")
	shouldApplyExponents := false
	if shouldApplyExponentsStr != "" {
		shouldApplyExponents, err = strconv.ParseBool(shouldApplyExponentsStr)
		if err != nil {
			return err
		}
	}

	tokenOutDenom, tokenIn, err := getValidRoutingParameters(c)
	if err != nil {
		return err
	}

	// translate denoms from human to chain if needed
	tokenOutDenom, tokenInDenom, err := a.getChainDenoms(c, tokenOutDenom, tokenIn.Denom)
	if err != nil {
		return err
	}

	// Update coins token in denom it case it was translated from human to chain.
	tokenIn.Denom = tokenInDenom

	var quote domain.Quote
	if isSingleRoute {
		quote, err = a.RUsecase.GetBestSingleRouteQuote(ctx, tokenIn, tokenOutDenom)
	} else {
		quote, err = a.RUsecase.GetOptimalQuote(ctx, tokenIn, tokenOutDenom)
	}
	if err != nil {
		return err
	}

	scalingFactor := oneDec
	if shouldApplyExponents {
		scalingFactor = a.getSpotPriceScalingFactor(tokenInDenom, tokenOutDenom)
	}

	_, _, err = quote.PrepareResult(ctx, scalingFactor)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.Stringer("token_out", quote.GetAmountOut()))
	span.SetAttributes(attribute.Stringer("price_impact", quote.GetPriceImpact()))

	return c.JSON(http.StatusOK, quote)
}

// @Summary Compute the quote for the given poolID
// @Description Call does not search for the route rather directly computes the quote for the given poolID.
// @ID get-direct-quote
// @Produce  json
// @Param  tokenIn         query  string  true  "String representation of the sdk.Coin for the token in."                   example(5OSMO)
// @Param  tokenOutDenom   query  string  true  "String representing the list of the token denom out separated by comma."   example(ATOM,USDC)
// @Param  poolID          query  string  true  "String representing list of the pool ID."                                  example(1,2,3)
// @Param  applyExponents  query  bool    false  "Boolean flag indicating whether to apply exponents to the spot price. False by default."
// @Success 200  {object}  domain.Quote  "The computed best route quote"
// @Router /router/custom-direct-quote [get]
func (a *RouterHandler) GetDirectCustomQuote(c echo.Context) error {
	ctx := c.Request().Context()

	poolIDs, tokenOutDenom, tokenIn, err := getDirectCustomQuoteParameters(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, domain.ResponseError{Message: err.Error()})
	}

	shouldApplyExponentsStr := c.QueryParam("applyExponents")
	shouldApplyExponents := false
	if shouldApplyExponentsStr != "" {
		shouldApplyExponents, err = strconv.ParseBool(shouldApplyExponentsStr)
		if err != nil {
			return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
		}
	}

	// Quote
	quote, err := a.RUsecase.GetCustomDirectQuoteMultiPool(ctx, tokenIn, tokenOutDenom, poolIDs)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

	scalingFactor := oneDec
	if shouldApplyExponents {
		// TODO: is this right approach to take last token out?
		scalingFactor = a.getSpotPriceScalingFactor(tokenIn.Denom, tokenOutDenom[len(tokenOutDenom)-1])
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

	// translate denoms from human to chain if needed
	tokenOutDenom, tokenIn, err = a.getChainDenoms(c, tokenOutDenom, tokenIn)
	if err != nil {
		return c.JSON(domain.GetStatusCode(err), domain.ResponseError{Message: err.Error()})
	}

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

// returns chain denoms from echo parameters. If human denoms are given, they are converted to chain denoms.
func (a *RouterHandler) getChainDenoms(c echo.Context, tokenOutDenom, tokenInDenom string) (string, string, error) {
	isHumanDenomsStr := c.QueryParam("humanDenoms")
	isHumanDenoms := false
	var err error
	if len(isHumanDenomsStr) > 0 {
		isHumanDenoms, err = strconv.ParseBool(isHumanDenomsStr)
		if err != nil {
			return "", "", err
		}
	}

	// Note that sdk.Coins initialization
	// auto-converts base denom from human
	// to IBC notation.
	// As a result, we avoid attempting the
	// to convert a denom that is already changed.
	baseDenom, err := sdk.GetBaseDenom()
	if err != nil {
		return "", "", nil
	}

	if isHumanDenoms {
		// See definition of baseDenom.
		if tokenOutDenom != baseDenom {
			tokenOutDenom, err = a.TUsecase.GetChainDenom(tokenOutDenom)
			if err != nil {
				return "", "", err
			}
		}

		// See definition of baseDenom.
		if tokenInDenom != baseDenom {
			tokenInDenom, err = a.TUsecase.GetChainDenom(tokenInDenom)
			if err != nil {
				return "", "", err
			}
		}
	} else {
		if !a.TUsecase.IsValidChainDenom(tokenInDenom) {
			return "", "", fmt.Errorf("tokenInDenom is not a valid chain denom (%s)", tokenInDenom)
		}

		if !a.TUsecase.IsValidChainDenom(tokenOutDenom) {
			return "", "", fmt.Errorf("tokenOutDenom is not a valid chain denom (%s)", tokenOutDenom)
		}
	}

	return tokenOutDenom, tokenInDenom, nil
}

// getValidRoutingParameters returns the tokenIn and tokenOutDenom from server context if they are valid.
func getValidRoutingParameters(c echo.Context) (string, sdk.Coin, error) {
	tokenOutStr, tokenInStr, err := getValidTokenInTokenOutStr(c)
	if err != nil {
		return "", sdk.Coin{}, err
	}

	tokenIn, err := sdk.ParseCoinNormalized(tokenInStr)
	if err != nil {
		return "", sdk.Coin{}, ErrTokenNotValid
	}

	return tokenOutStr, tokenIn, nil
}

// getDirectCustomQuoteParameters returns the pool IDs, tokenIn and tokenOutDenom from server context if they are valid.
func getDirectCustomQuoteParameters(c echo.Context) ([]uint64, []string, sdk.Coin, error) {
	poolID, tokenOut, tokenInStr, err := getPoolsValidTokenInTokensOut(c)
	if err != nil {
		return nil, nil, sdk.Coin{}, err
	}

	tokenIn, err := sdk.ParseCoinNormalized(tokenInStr)
	if err != nil {
		return nil, nil, sdk.Coin{}, ErrTokenNotValid
	}

	return poolID, tokenOut, tokenIn, nil
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
		return "", ErrTokenNotSpecified
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

	// Validate input denoms
	if err := domain.ValidateInputDenoms(tokenInStr, tokenOutStr); err != nil {
		return "", "", err
	}

	return tokenOutStr, tokenInStr, nil
}

func getValidPoolID(c echo.Context) ([]uint64, error) {
	// We accept two poolIDs and poolID parameters, and require at least one of them to be filled
	poolIDStr := strings.Split(c.QueryParam("poolID"), ",")
	if len(poolIDStr) == 0 {
		return nil, errors.New("poolID is required")
	}

	var poolIDs []uint64
	for _, v := range poolIDStr {
		i, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return nil, err
		}
		poolIDs = append(poolIDs, i)
	}

	return poolIDs, nil
}

func getPoolsValidTokenInTokensOut(c echo.Context) (poolIDs []uint64, tokenOut []string, tokenIn string, err error) {
	poolIDs, err = getValidPoolID(c)
	if err != nil {
		return nil, nil, "", err
	}

	tokenIn, err = getValidTokenInStr(c)
	if err != nil {
		return nil, nil, "", err
	}

	tokenOut = strings.Split(c.QueryParam("tokenOutDenom"), ",")
	if len(tokenOut) == 0 {
		return nil, nil, "", errors.New("tokenOutDenom is required")
	}

	// one output per each pool
	if len(tokenOut) != len(poolIDs) {
		return nil, nil, "", ErrNumOfTokenOutDenomPoolsMismatch
	}

	// Validate denoms
	for _, v := range tokenOut {
		if err := domain.ValidateInputDenoms(tokenIn, v); err != nil {
			return nil, nil, "", err
		}
	}

	return poolIDs, tokenOut, tokenIn, nil
}
