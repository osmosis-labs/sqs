package orderbookdomain

import (
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

type OrderbookTick struct {
	Tick              *cosmwasmpool.OrderbookTick
	TickState         TickState
	UnrealizedCancels UnrealizedCancels
}

type UnrealizedCancels struct {
	AskUnrealizedCancels osmomath.Int `json:"ask_unrealized_cancels"`
	BidUnrealizedCancels osmomath.Int `json:"bid_unrealized_cancels"`
}

type Tick struct {
	TickID    int64     `json:"tick_id"`
	TickState TickState `json:"tick_state"`
}

type TickState struct {
	AskValues TickValues `json:"ask_values"`
	BidValues TickValues `json:"bid_values"`
}

type TickValues struct {
	// Total Amount of Liquidity at tick (TAL)
	// - Every limit order placement increments this value.
	// - Every swap at this tick decrements this value.
	// - Every cancellation decrements this value.
	TotalAmountOfLiquidity string `json:"total_amount_of_liquidity"`

	// Cumulative Total Limits at tick (CTT)
	// - Every limit order placement increments this value.
	// - There might be an edge-case optimization to lower this value.
	CumulativeTotalValue string `json:"cumulative_total_value"`

	// Effective Total Amount Swapped at tick (ETAS)
	// - Every swap increments ETAS by the swap amount.
	// - There will be other ways to update ETAS as described below.
	EffectiveTotalAmountSwapped string `json:"effective_total_amount_swapped"`

	// Cumulative Realized Cancellations at tick
	// - Increases as cancellations are checkpointed in batches on the sumtree
	// - Equivalent to the prefix sum at the tick's current ETAS after being synced
	CumulativeRealizedCancels string `json:"cumulative_realized_cancels"`

	// last_tick_sync_etas is the ETAS value after the most recent tick sync.
	// It is used to skip tick syncs if ETAS has not changed since the previous
	// sync.
	LastTickSyncEtas string `json:"last_tick_sync_etas"`
}

// isTickFullyFilled checks if a tick is fully filled by comparing its cumulative total value
// to its effective total amount swapped.
func (tv *TickValues) IsTickFullyFilled() (bool, error) {
	cumulativeTotalValue, err := osmomath.NewDecFromStr(tv.CumulativeTotalValue)
	if err != nil {
		return false, err
	}

	effectiveTotalAmountSwapped, err := osmomath.NewDecFromStr(tv.EffectiveTotalAmountSwapped)
	if err != nil {
		return false, err
	}

	return cumulativeTotalValue.Equal(effectiveTotalAmountSwapped), nil
}
