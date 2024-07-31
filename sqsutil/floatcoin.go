package sqsutil

import (
	"math"
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

func NewFloatCoinFromSdkCoin(coin sdk.Coin) FloatCoin {
	return NewFloatCoinFromSdkCoinWithScratch(new(big.Float), coin)
}

func NewFloatCoinFromSdkCoinWithScratch(scratch *big.Float, coin sdk.Coin) FloatCoin {
	bi := coin.Amount.BigIntMut()
	fAmount := bigIntToFloat(scratch, bi, RoundUnspecified)
	return FloatCoin{Denom: coin.Denom, Amount: fAmount}
}

type RoundingMode int

const (
	RoundDown RoundingMode = iota
	RoundUp
	RoundUnspecified
)

// For use when the caller knows the input is sorted by denom, and wants to avoid resorting.
// Should be used when the caller is sure that the input is sorted by denom. (e.g. valid SDK.coins)
type FloatCoinsList []FloatCoin

func FloatCoinsListToSdkCoins(coinsList FloatCoinsList, r RoundingMode) sdk.Coins {
	coins := make([]sdk.Coin, 0, len(coinsList))
	scratchFloat := new(big.Float)
	for _, coin := range coinsList {
		amt := floatToSDKInt(scratchFloat, coin.Amount, r)
		if amt.Sign() != 0 {
			coins = append(coins, sdk.Coin{Denom: coin.Denom, Amount: amt})
		}
	}
	return coins
}

func FloatCoinsListFromSDKCoins(coins sdk.Coins) FloatCoinsList {
	coinsList := make([]FloatCoin, 0, len(coins))
	scratchFloat := new(big.Float)
	for _, coin := range coins {
		NewFloatCoinFromSdkCoinWithScratch(scratchFloat, coin)
		coinsList = append(coinsList, NewFloatCoinFromSdkCoinWithScratch(scratchFloat, coin))
	}
	return coinsList
}

type FloatCoinsMap struct {
	*btree.Map[string, float64]
}

func NewFloatCoinsMap() *FloatCoinsMap {
	return &FloatCoinsMap{btree.NewMap[string, float64](4)}
}

func NewFloatCoinsMapFromCoins(coins sdk.Coins) *FloatCoinsMap {
	m := NewFloatCoinsMap()
	scratch := new(big.Float)
	for _, coin := range coins {
		bi := coin.Amount.BigIntMut()
		fAmount := bigIntToFloat(scratch, bi, RoundUnspecified)
		m.Load(coin.Denom, fAmount)
	}
	return m
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

//nolint:unparam
func bigIntToFloat(scratchFloat *big.Float, amount *big.Int, roundingMode RoundingMode) float64 {
	fAmount, _ := scratchFloat.SetInt(amount).Float64()
	// TODO: implement rounding mode, not so obvious how to do this
	// switch roundingMode {
	// case RoundDown:
	// 	// do nothing, rounds down by default
	// case RoundUp:
	// 	if amount.Sign() > 0 {
	// 		fAmount += 1
	// 	}
	// case RoundUnspecified:
	// 	// No rounding specified, use the default rounding mode
	// }
	return fAmount
}

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

var maxFloatForInt = math.Exp2(256.0) - 1.0
var maxSdkInt sdkmath.Int

func init() {
	maxSdkInt = sdkmath.OneInt()
	maxSdkInt.BigIntMut().Lsh(maxSdkInt.BigIntMut(), 256)
	maxSdkInt = maxSdkInt.Sub(sdkmath.OneInt())
	if maxSdkInt.BigInt().BitLen() > sdkmath.MaxBitLen {
		panic("maxSdkInt is too large")
	}
}

func floatToSDKInt(scratchFloat *big.Float, amount float64, roundingMode RoundingMode) sdkmath.Int {
	if amount > maxFloatForInt {
		// If the amount is too large, return the maximum possible integer
		return maxSdkInt
	}
	amt := floatToBigInt(scratchFloat, amount, roundingMode)
	amtInt := sdkmath.NewIntFromBigIntMut(amt)
	return amtInt
}

func (m *FloatCoinsMap) ToSdkCoins(r RoundingMode) sdk.Coins {
	coins := make([]sdk.Coin, 0, m.Len())
	scratchFloat := new(big.Float)
	m.Scan(func(denom string, amount float64) bool {
		amt := floatToSDKInt(scratchFloat, amount, r)
		if amt.Sign() != 0 {
			coins = append(coins, sdk.Coin{Denom: denom, Amount: amt})
		}
		return true
	})
	return coins
}
