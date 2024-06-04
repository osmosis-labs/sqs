package http

func (a *RouterHandler) GetMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom string, disableMinLiquidityFallback bool) (uint64, error) {
	return a.getMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom, disableMinLiquidityFallback)
}
