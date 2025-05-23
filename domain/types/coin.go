package types

import (
	"math/big"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// NewCoin returns a new coin with a denomination and amount.
func NewCoin(denom string, amount osmomath.Int) Coin {
	return Coin{
		Denom:       denom,
		AmountInt:   amount,
		AmountFloat: *new(big.Float).SetInt(amount.BigInt()),
	}
}

// Coins is a set of Coin, one per currency
type Coins []Coin

// Coin defines a token with a denomination and an amount.
type Coin struct {
	Denom       string
	AmountInt   osmomath.Int
	AmountFloat big.Float
}
