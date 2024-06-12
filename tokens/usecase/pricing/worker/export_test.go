package worker

type PoolLiquidityPricerWorker = poolLiquidityPricerWorker

const (
	LiquidityCapErrorSeparator = liquidityCapErrorSeparator
)

func (p *poolLiquidityPricerWorker) HasLaterUpdateThanHeight(denom string, height uint64) bool {
	return p.hasLaterUpdateThanHeight(denom, height)
}

func FormatLiquidityCapErrorStr(denom string) string {
	return formatLiquidityCapErrorStr(denom)
}
