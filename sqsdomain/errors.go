package sqsdomain

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type ConcentratedPoolNoTickModelError struct {
	PoolId uint64
}

func (e ConcentratedPoolNoTickModelError) Error() string {
	return fmt.Sprintf("concentrated pool (%d) has no tick model", e.PoolId)
}

type OrderbookPoolInvalidDirectionError struct {
	Direction int64
}

func (e OrderbookPoolInvalidDirectionError) Error() string {
	return fmt.Sprintf("orderbook pool direction (%d) is invalid; must be either -1 or 1", e.Direction)
}

type OrderbookNotEnoughLiquidityToCompleteSwapError struct {
	PoolId   uint64
	AmountIn sdk.Coin
}

func (e OrderbookNotEnoughLiquidityToCompleteSwapError) Error() string {
	return fmt.Sprintf("not enough liquidity to complete swap in pool (%d) with amount in (%s)", e.PoolId, e.AmountIn)
}

type OrderbookPoolMismatchError struct {
	PoolId        uint64
	TokenInDenom  string
	TokenOutDenom string
}

func (e OrderbookPoolMismatchError) Error() string {
	return fmt.Sprintf("orderbook pool (%d) does not support swaps from (%s) to (%s)", e.PoolId, e.TokenInDenom, e.TokenOutDenom)
}
