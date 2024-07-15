package usecase

import "github.com/osmosis-labs/osmosis/osmomath"

func (p *poolsUseCase) ProcessOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom string, poolID uint64, poolLiquidityCapitalization osmomath.Int) (updatedBool bool, err error) {
	return p.processOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom, poolID, poolLiquidityCapitalization)
}
