package http

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/json"
)

// ResponseError represent the response error struct
type ResponseError struct {
	Message string `json:"message"`
}

// RouterHandler  represent the httphandler for the router
type RouterHandler struct {
	RUsecase mvc.RouterUsecase
	logger   log.Logger
}

const routerResource = "/router"

func formatRouterResource(resource string) string {
	return routerResource + resource
}

// NewRouterHandler will initialize the pools/ resources endpoint
func NewRouterHandler(e *echo.Echo, us mvc.RouterUsecase, logger log.Logger) {
	handler := &RouterHandler{
		RUsecase: us,
		logger:   logger,
	}
	e.GET(formatRouterResource("/quote"), handler.GetOptimalQuote)
	e.GET(formatRouterResource("/single-quote"), handler.GetBestSingleRouteQuote)
	e.GET(formatRouterResource("/routes"), handler.GetCandidateRoutes)
	e.GET(formatRouterResource("/cached-routes"), handler.GetCachedCandidateRoutes)
	e.GET(formatRouterResource("/custom-direct-quote"), handler.GetDirectCustomQuote)
	e.GET(formatRouterResource("/custom-quote"), handler.GetCustomQuote)
	e.GET(formatRouterResource("/taker-fee-pool/:id"), handler.GetTakerFee)
	e.POST(formatRouterResource("/store-state"), handler.StoreRouterStateInFiles)
	e.POST(formatRouterResource("/overwrite-route"), handler.OverwriteRoute)
}

// GetOptimalQuote will determine the optimal quote for a given tokenIn and tokenOutDenom
// Return the optimal quote.
func (a *RouterHandler) GetOptimalQuote(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidRoutingParameters(c)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	quote, err := a.RUsecase.GetOptimalQuote(ctx, tokenIn, tokenOutDenom)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	quote.PrepareResult()

	err = c.JSON(http.StatusOK, quote)
	if err != nil {
		return err
	}

	return nil
}

// GetBestSingleRouteQuote returns the best single route quote to be done directly without a split.
func (a *RouterHandler) GetBestSingleRouteQuote(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidRoutingParameters(c)
	if err != nil {
		return err
	}

	quote, err := a.RUsecase.GetBestSingleRouteQuote(ctx, tokenIn, tokenOutDenom)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	quote.PrepareResult()

	return c.JSON(http.StatusOK, quote)
}

// GetCustomQuote returns a direct custom quote. It ensures that the route contains all the pools
// listed in the specific order, returns error if such route is not found.
func (a *RouterHandler) GetCustomQuote(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidRoutingParameters(c)
	if err != nil {
		return err
	}

	poolIDsStr := c.QueryParam("poolIDs")
	if len(poolIDsStr) == 0 {
		return c.JSON(http.StatusBadRequest, ResponseError{Message: "poolIDs is required"})
	}

	poolIDs, err := parseNumbers(poolIDsStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ResponseError{Message: err.Error()})
	}

	// Quote
	quote, err := a.RUsecase.GetCustomQuote(ctx, tokenIn, tokenOutDenom, poolIDs)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	quote.PrepareResult()

	return c.JSON(http.StatusOK, quote)
}

// GetDirectCustomQuote returns a direct custom quote. It does not search for the route.
// It directly computes the quote for the given poolID.
func (a *RouterHandler) GetDirectCustomQuote(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidRoutingParameters(c)
	if err != nil {
		return err
	}

	poolIDStr := c.QueryParam("poolID")
	if len(poolIDStr) == 0 {
		return c.JSON(http.StatusBadRequest, ResponseError{Message: "poolID is required"})
	}

	poolID, err := strconv.ParseUint(poolIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ResponseError{Message: err.Error()})
	}

	// Quote
	quote, err := a.RUsecase.GetCustomDirectQuote(ctx, tokenIn, tokenOutDenom, poolID)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	quote.PrepareResult()

	return c.JSON(http.StatusOK, quote)
}

// GetCandidateRoutes returns the candidate routes for a given tokenIn and tokenOutDenom
func (a *RouterHandler) GetCandidateRoutes(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidTokenInTokenOutStr(c)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	routes, err := a.RUsecase.GetCandidateRoutes(ctx, tokenIn, tokenOutDenom)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	if err := c.JSON(http.StatusOK, routes); err != nil {
		return err
	}
	return nil
}

func (a *RouterHandler) GetTakerFee(c echo.Context) error {
	ctx := c.Request().Context()

	idStr := c.Param("id")
	poolID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ResponseError{Message: err.Error()})
	}

	takerFees, err := a.RUsecase.GetTakerFee(ctx, poolID)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, takerFees)
}

// GetCandidateRoutes returns the candidate routes for a given tokenIn and tokenOutDenom from cache.
// If no routes present in cache, it does not attempt to recompute them.
func (a *RouterHandler) GetCachedCandidateRoutes(c echo.Context) error {
	ctx := c.Request().Context()

	tokenOutDenom, tokenIn, err := getValidTokenInTokenOutStr(c)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	routes, err := a.RUsecase.GetCachedCandidateRoutes(ctx, tokenIn, tokenOutDenom)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, routes)
}

// TODO: authentication for the endpoint and enable only in dev mode.
func (a *RouterHandler) StoreRouterStateInFiles(c echo.Context) error {
	ctx := c.Request().Context()

	if err := a.RUsecase.StoreRouterStateFiles(ctx); err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, "Router state stored in files")
}

// TODO: authentication for the endpoint and enable only in dev mode.
func (a *RouterHandler) OverwriteRoute(c echo.Context) error {
	ctx := c.Request().Context()

	// Get the tokenInDenom denom string
	tokenInDenom, err := getValidTokenInStr(c)
	if err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	// Read the request body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Error reading request body")
	}

	// Parse the request body
	var routes []sqsdomain.CandidateRoute
	if err := json.Unmarshal(body, &routes); err != nil {
		return c.String(http.StatusInternalServerError, "Error parsing request body")
	}

	if err := a.RUsecase.OverwriteRoutes(ctx, tokenInDenom, routes); err != nil {
		return c.JSON(getStatusCode(err), ResponseError{Message: err.Error()})
	}

	return c.JSON(http.StatusOK, "Router state stored in files")
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

// parseNumbers parses a comma-separated list of numbers into a slice of unit64.
func parseNumbers(numbersParam string) ([]uint64, error) {
	var numbers []uint64
	numStrings := splitAndTrim(numbersParam, ",")

	for _, numStr := range numStrings {
		num, err := strconv.ParseUint(numStr, 10, 64)
		if err != nil {
			return nil, err
		}
		numbers = append(numbers, num)
	}

	return numbers, nil
}

// splitAndTrim splits a string by a separator and trims the resulting strings.
func splitAndTrim(s, sep string) []string {
	var result []string
	for _, val := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
