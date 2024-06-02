package worker

type PoolLiquidityPricerWorker = poolLiquidityPricerWorker

func (p *poolLiquidityPricerWorker) HasLaterUpdateThanHeight(denom string, height uint64) bool {
	return p.hasLaterUpdateThanHeight(denom, height)
}

func (p *poolLiquidityPricerWorker) StoreHeightForDenom(denom string, height uint64) {
	p.storeHeightForDenom(denom, height)
}
