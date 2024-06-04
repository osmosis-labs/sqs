package http

func (a *RouterHandler) GetMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom string, disableMinLiquidityCapFallback, forceDefaultMinLiquidityCapFilter bool) (uint64, error) {
	return a.getMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom, disableMinLiquidityCapFallback, forceDefaultMinLiquidityCapFilter)
}
