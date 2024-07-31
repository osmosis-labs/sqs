package sqsutil

import (
	"math/big"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/tidwall/btree"
)

type FloatCoin struct {
	// The name of the coin
	Denom string `json:"denom"`
	// The symbol of the coin
	Amount float64 `json:"amount"`
}

type RoundingMode int

const (
	RoundDown RoundingMode = iota
	RoundUp
	RoundUnspecified
)

type FloatCoinsMap struct {
	*btree.Map[string, float64]
}

func NewFloatCoinsMap() *FloatCoinsMap {
	return &FloatCoinsMap{btree.NewMap[string, float64](4)}
}

// Mutative methods
func (m *FloatCoinsMap) Add(denom string, amount float64) {
	amt, ok := m.Get(denom)
	newAmt := amount
	if ok {
		newAmt = amt + amount
	}

	if newAmt == 0 {
		m.Delete(denom)
	} else {
		m.Set(denom, newAmt)
	}
}

// Mutative
func (m *FloatCoinsMap) Sub(denom string, amount float64) {
	amt, ok := m.Get(denom)
	newAmt := -amount
	if ok {
		newAmt = amt - amount
	}

	if newAmt == 0 {
		m.Delete(denom)
	} else {
		m.Set(denom, newAmt)
	}
}

func (m *FloatCoinsMap) ToSortedList() []FloatCoin {
	coins := make([]FloatCoin, 0, m.Len())
	m.Scan(func(denom string, amount float64) bool {
		coins = append(coins, FloatCoin{Denom: denom, Amount: amount})
		return true
	})
	return coins
}

var oneInt = big.NewInt(1)

func floatToBigInt(scratchFloat *big.Float, amount float64, roundingMode RoundingMode) *big.Int {
	bigAmount := scratchFloat.SetFloat64(amount)

	bigInt := new(big.Int)
	bigInt, accuracy := bigAmount.Int(bigInt)

	switch roundingMode {
	case RoundDown:
		// do nothing, rounds down by default
	case RoundUp:
		if accuracy != big.Exact {
			bigInt.Add(bigInt, oneInt)
		}
	case RoundUnspecified:
		// No rounding specified, use the default rounding mode
	}

	return bigInt
}

func (m *FloatCoinsMap) ToSdkCoins(r RoundingMode) sdk.Coins {
	coins := make([]sdk.Coin, 0, m.Len())
	scratchFloat := new(big.Float)
	m.Scan(func(denom string, amount float64) bool {
		amt := floatToBigInt(scratchFloat, amount, r)
		amtInt := sdkmath.NewIntFromBigIntMut(amt)
		if amt.Sign() != 0 {
			coins = append(coins, sdk.Coin{Denom: denom, Amount: amtInt})
		}
		return true
	})
	return coins
}
