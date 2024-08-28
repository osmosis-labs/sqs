package sqsutil

import (
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	math "github.com/osmosis-labs/osmosis/osmomath"
)

func baseFloatCoinsMap() *FloatCoinsMap {
	// Create a new FloatCoinsMap
	fcm := NewFloatCoinsMap()

	// Add some coins to the map
	fcm.Add("BTC", 1.5)
	fcm.Add("ETH", 2.3)
	fcm.Add("SOL", 3)
	fcm.Add("XRP", 0.8)
	return fcm
}

func TestFloatCoinsMap_ToSortedList(t *testing.T) {
	// Create a new FloatCoinsMap
	fcm := baseFloatCoinsMap()

	// Get the sorted list of coins
	coins := fcm.ToSortedList()

	// Check the length of the sorted list
	if len(coins) != 4 {
		t.Errorf("Expected sorted list length to be 3, got %d", len(coins))
	}

	// Check the order of the coins in the sorted list
	expectedCoins := []FloatCoin{
		{Denom: "BTC", Amount: 1.5},
		{Denom: "ETH", Amount: 2.3},
		{Denom: "SOL", Amount: 3},
		{Denom: "XRP", Amount: 0.8},
	}
	for i, coin := range coins {
		if coin != expectedCoins[i] {
			t.Errorf("Expected coin at index %d to be %v, got %v", i, expectedCoins[i], coin)
		}
	}
}
func TestFloatCoinsMap_ToSdkCoins(t *testing.T) {
	tests := []struct {
		name          string
		coinsMap      *FloatCoinsMap
		expectedCoins []sdk.Coin
		rounding      RoundingMode
	}{
		{
			name:     "Round down",
			coinsMap: baseFloatCoinsMap(),
			expectedCoins: []sdk.Coin{
				{Denom: "BTC", Amount: math.NewInt(1)},
				{Denom: "ETH", Amount: math.NewInt(2)},
				{Denom: "SOL", Amount: math.NewInt(3)},
			},
			rounding: RoundDown,
		},
		{
			name:     "Round up",
			coinsMap: baseFloatCoinsMap(),
			expectedCoins: []sdk.Coin{
				{Denom: "BTC", Amount: math.NewInt(2)},
				{Denom: "ETH", Amount: math.NewInt(3)},
				{Denom: "SOL", Amount: math.NewInt(3)},
				{Denom: "XRP", Amount: math.NewInt(1)},
			},
			rounding: RoundUp,
		}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coins := tt.coinsMap.ToSdkCoins(tt.rounding)

			if len(coins) != len(tt.expectedCoins) {
				t.Errorf("Expected SDK coins length to be %d, got %d", len(tt.expectedCoins), len(coins))
			}

			for i, coin := range coins {
				if !coin.Equal(tt.expectedCoins[i]) {
					t.Errorf("Expected SDK coin at index %d to be %v, got %v", i, tt.expectedCoins[i], coin)
				}
			}
		})
	}
}

func BenchmarkFloatCoinsMapAdd(b *testing.B) {
	numCoins := 10
	coins := sdk.Coins{}
	for i := 0; i < numCoins; i++ {
		coins = append(coins, sdk.NewInt64Coin(fmt.Sprintf("coin%04d", i), 1))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fcm := NewFloatCoinsMap()
		for _, coin := range coins {
			fCoin := NewFloatCoinFromSdkCoin(coin)
			fcm.Add(coin.Denom, fCoin.Amount)
		}
	}
}

func BenchmarkFloatCoinsMapFromCoins(b *testing.B) {
	numCoins := 10
	coins := sdk.Coins{}
	for i := 0; i < numCoins; i++ {
		coins = append(coins, sdk.NewInt64Coin(fmt.Sprintf("coin%04d", i), 1))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fcm := NewFloatCoinsMapFromCoins(coins)
		_ = fcm
	}
}
