package usecase

import (
	"github.com/osmosis-labs/osmosis/osmomath"
)

type (
	OrderBookEntry = orderBookEntry
	PoolsUsecase   = poolsUseCase
)

const (
	OriginalOrderbookAddress = "original-address"
)

func (p *poolsUseCase) ProcessOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom string, poolID uint64, poolLiquidityCapitalization osmomath.Int, contractAddress string) (updatedBool bool, err error) {
	return p.processOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom, poolID, poolLiquidityCapitalization, contractAddress)
}

// WARNING: this method is only meant for setting up tests. Do not move out of export_test.go
func (p *poolsUseCase) StoreValidOrdeBookEntry(baseDenom, quoteDenom string, poolID uint64, poolLiquidityCapitalization osmomath.Int) {
	p.canonicalOrderBookForBaseQuoteDenom.Store(formatBaseQuoteDenom(baseDenom, quoteDenom), orderBookEntry{
		PoolID:       poolID,
		LiquidityCap: poolLiquidityCapitalization,
		ContractAddress: OriginalOrderbookAddress,
	})
}

// WARNING: this method is only meant for setting up tests. Do not move out of export_test.go
func (p *poolsUseCase) StoreInvalidOrderBookEntry(baseDenom, quoteDenom string) {
	const invalidEntryType = 1
	p.canonicalOrderBookForBaseQuoteDenom.Store(formatBaseQuoteDenom(baseDenom, quoteDenom), invalidEntryType)
}
