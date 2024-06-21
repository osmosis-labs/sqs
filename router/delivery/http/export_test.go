package http

import "github.com/labstack/echo/v4"

func (a *RouterHandler) GetMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom string, disableMinLiquidityCapFallback, forceDefaultMinLiquidityCapFilter bool) (uint64, error) {
	return a.getMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom, disableMinLiquidityCapFallback, forceDefaultMinLiquidityCapFilter)
}

func GetPoolsValidTokenInTokensOut(c echo.Context) (poolIDs []uint64, tokenOut []string, tokenIn string, err error) {
	return getPoolsValidTokenInTokensOut(c)
}
