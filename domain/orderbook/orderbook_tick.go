package orderbookdomain

import (
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

type OrderbookTick struct {
	Tick              *cosmwasmpool.OrderbookTick
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

type TickValues struct {
	TotalAmountOfLiquidity      string `json:"total_amount_of_liquidity"`
	CumulativeTotalValue        string `json:"cumulative_total_value"`
	EffectiveTotalAmountSwapped string `json:"effective_total_amount_swapped"`
	CumulativeRealizedCancels   string `json:"cumulative_realized_cancels"`
	LastTickSyncEtas            string `json:"last_tick_sync_etas"`
}

type TickState struct {
	AskValues TickValues `json:"ask_values"`
	BidValues TickValues `json:"bid_values"`
}
