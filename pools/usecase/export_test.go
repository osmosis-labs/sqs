package usecase

import (
	"github.com/osmosis-labs/osmosis/osmomath"
)

type OrderBookEntry = orderBookEntry

func (p *poolsUseCase) ProcessOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom string, poolID uint64, poolLiquidityCapitalization osmomath.Int) (updatedBool bool, err error) {
	return p.processOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom, poolID, poolLiquidityCapitalization)
}

// WARNING: this method is only meant for setting up tests. Do not move out of exporte_test.go
func (p *poolsUseCase) StoreValidOrdeBookEntry(baseDenom, quoteDenom string, poolID uint64, poolLiquidityCapitalization osmomath.Int) {
	p.canonicalOrderBookForBaseQuoteDenom.Store(formatBaseQuoteDenom(baseDenom, quoteDenom), orderBookEntry{
		PoolID:       poolID,
		LiquidityCap: poolLiquidityCapitalization,
	})
}

// WARNING: this method is only meant for setting up tests. Do not move out of exporte_test.go
func (p *poolsUseCase) StoreInvalidOrdeBookEntry(baseDenom, quoteDenom string) {
	const invalidEntryType = 1
	p.canonicalOrderBookForBaseQuoteDenom.Store(formatBaseQuoteDenom(baseDenom, quoteDenom), invalidEntryType)
}
