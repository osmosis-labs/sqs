package domain_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
)

// The contract never lets a corrupted asset increase in amount or weight, so swapping a corrupted denom
// in is rejected while swapping it out is allowed. For an alloyed transmuter, swapping the alloyed asset
// in removes an asset from the pool and raises the weight of every other corrupted asset that holds a
// balance, unless the swap redeems a corrupted asset's whole balance.
func TestTransmuterStatuses_CheckSwapCorruptedDirection(t *testing.T) {
	const (
		poolID     = uint64(7)
		corruptedA = "ibc/CORRUPTA"
		corruptedB = "ibc/CORRUPTB"
		healthy    = "ibc/HEALTHY"
		alloyed    = "factory/alloy"
	)

	balances := sdk.NewCoins(sdk.NewInt64Coin(corruptedA, 100), sdk.NewInt64Coin(corruptedB, 100), sdk.NewInt64Coin(healthy, 100))
	emptyB := sdk.NewCoins(sdk.NewInt64Coin(corruptedA, 100), sdk.NewInt64Coin(healthy, 100))

	tests := map[string]struct {
		corrupted         []string
		tokenIn, tokenOut string
		alloyedDenom      string
		outAmount         int64
		balances          sdk.Coins
		wantErr           bool
	}{
		"corrupted in is blocked":                        {corrupted: []string{corruptedA}, tokenIn: corruptedA, tokenOut: healthy, balances: balances, wantErr: true},
		"corrupted out is allowed":                       {corrupted: []string{corruptedA}, tokenIn: healthy, tokenOut: corruptedA, balances: balances},
		"healthy pair is allowed":                        {corrupted: []string{corruptedA}, tokenIn: healthy, tokenOut: "ibc/OTHER", balances: balances},
		"alloyed in to healthy out is blocked":           {corrupted: []string{corruptedA}, tokenIn: alloyed, tokenOut: healthy, alloyedDenom: alloyed, outAmount: 10, balances: balances, wantErr: true},
		"alloyed in to only corrupted out is allowed":    {corrupted: []string{corruptedA}, tokenIn: alloyed, tokenOut: corruptedA, alloyedDenom: alloyed, outAmount: 10, balances: balances},
		"healthy in to alloyed out is allowed":           {corrupted: []string{corruptedA, corruptedB}, tokenIn: healthy, tokenOut: alloyed, alloyedDenom: alloyed, balances: balances},
		"healthy in to one of two corrupted is allowed":  {corrupted: []string{corruptedA, corruptedB}, tokenIn: healthy, tokenOut: corruptedA, balances: balances},
		"v1 pool never treats a denom as the alloyed":    {corrupted: []string{corruptedA}, tokenIn: alloyed, tokenOut: healthy, balances: balances},
		"alloyed in to one of two corrupted is blocked":  {corrupted: []string{corruptedA, corruptedB}, tokenIn: alloyed, tokenOut: corruptedA, alloyedDenom: alloyed, outAmount: 10, balances: balances, wantErr: true},
		"two corrupted, full balance redemption allowed": {corrupted: []string{corruptedA, corruptedB}, tokenIn: alloyed, tokenOut: corruptedA, alloyedDenom: alloyed, outAmount: 100, balances: balances},
		"two corrupted, other one drained is allowed":    {corrupted: []string{corruptedA, corruptedB}, tokenIn: alloyed, tokenOut: corruptedA, alloyedDenom: alloyed, outAmount: 10, balances: emptyB},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			statuses := domain.NewTransmuterStatuses()
			statuses.SetCorruptedDenoms(poolID, tc.corrupted)

			err := statuses.CheckSwap(poolID, tc.tokenIn, tc.tokenOut, tc.alloyedDenom, math.NewInt(tc.outAmount), tc.balances)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// A transmuter that the contract reports as paused rejects every swap, and a pool without a stored status is routed.
func TestTransmuterStatuses_CheckSwapInactive(t *testing.T) {
	const poolID = uint64(7)

	check := func(statuses *domain.TransmuterStatuses) error {
		return statuses.CheckSwap(poolID, "a", "b", "", math.NewInt(1), nil)
	}

	statuses := domain.NewTransmuterStatuses()
	require.NoError(t, check(statuses))

	statuses.SetActive(poolID, false)
	require.ErrorAs(t, check(statuses), &domain.TransmuterInactiveError{})
	require.True(t, statuses.IsInactive(poolID))

	statuses.SetActive(poolID, true)
	require.NoError(t, check(statuses))
}
