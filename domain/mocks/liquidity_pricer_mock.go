package mocks

import (
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

type LiquidityPricerMock struct {
	PriceBalancesFunc func(balances types.Coins, blockPriceUpdates domain.PricesResult) (math.Int, string)
	PriceCoinFunc     func(liquidity types.Coin, price osmomath.BigDec) math.LegacyDec
}

// PriceBalances implements domain.LiquidityPricer.
func (l *LiquidityPricerMock) PriceBalances(balances types.Coins, blockPriceUpdates domain.PricesResult) (math.Int, string) {
	if l.PriceBalancesFunc != nil {
		return l.PriceBalancesFunc(balances, blockPriceUpdates)
	}
	panic("unimplemented")
}

// PriceCoin implements domain.LiquidityPricer.
func (l *LiquidityPricerMock) PriceCoin(liquidity types.Coin, price osmomath.BigDec) math.LegacyDec {
	if l.PriceCoinFunc != nil {
		return l.PriceCoinFunc(liquidity, price)
	}
	panic("unimplemented")
}

var _ domain.LiquidityPricer = &LiquidityPricerMock{}
