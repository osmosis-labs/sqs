package http

import "github.com/labstack/echo/v4"

func GetPoolsValidTokenInTokensOut(c echo.Context) (poolIDs []uint64, tokenOut []string, tokenIn string, err error) {
	return getPoolsValidTokenInTokensOut(c)
}
