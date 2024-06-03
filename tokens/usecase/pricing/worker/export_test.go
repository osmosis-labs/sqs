package worker

type PoolLiquidityPricerWorker = poolLiquidityPricerWorker

func (p *poolLiquidityPricerWorker) HasLaterUpdateThanHeight(denom string, height uint64) bool {
	return p.hasLaterUpdateThanHeight(denom, height)
}
