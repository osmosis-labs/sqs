package worker

import "github.com/osmosis-labs/sqs/domain"

type PoolLiquidityPricerWorker = poolLiquidityPricerWorker

const (
	LiquidityCapErrorSeparator = liquidityCapErrorSeparator
	GammSharePrefix            = gammSharePrefix
)

func (p *poolLiquidityPricerWorker) HasLaterUpdateThanHeight(denom string, height uint64) bool {
	return p.hasLaterUpdateThanHeight(denom, height)
}

func FormatLiquidityCapErrorStr(denom string) string {
	return formatLiquidityCapErrorStr(denom)
}

func (p *poolLiquidityPricerWorker) RepricePoolLiquidityCap(poolIDs map[uint64]struct{}, blockPriceUpdates domain.PricesResult) error {
	return p.repricePoolLiquidityCap(poolIDs, blockPriceUpdates)
}

func (p *poolLiquidityPricerWorker) ShouldSkipDenomRepricing(denom string, updateHeight uint64) bool {
	return p.shouldSkipDenomRepricing(denom, updateHeight)
}
