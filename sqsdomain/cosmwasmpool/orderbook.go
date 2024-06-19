package cosmwasmpool

import (
	"github.com/osmosis-labs/osmosis/osmomath"
)

const (
	ORDERBOOK_CONTRACT_NAME               = "crates.io:sumtree-orderbook"
	ORDERBOOK_CONTRACT_VERSION_CONSTRAINT = ">= 0.1.0"
)

func (model *CosmWasmPoolModel) IsOrderbook() bool {
	return model.ContractInfo.Matches(
		ORDERBOOK_CONTRACT_NAME,
		mustParseSemverConstraint(ORDERBOOK_CONTRACT_VERSION_CONSTRAINT),
	)
}

// OrderbookData, since v1.0.0
type OrderbookData struct {
	QuoteDenom  string                    `json:"quote_denom"`
	BaseDenom   string                    `json:"base_denom"`
	NextBidTick int64                     `json:"next_bid_tick"`
	NextAskTick int64                     `json:"next_ask_tick"`
	Ticks       []OrderbookTickIdAndState `json:"ticks"`
}

type OrderbookTickValues struct {
	// Total Amount of Liquidity at tick (TAL)
	// - Every limit order placement increments this value.
	// - Every swap at this tick decrements this value.
	// - Every cancellation decrements this value.
	TotalAmountOfLiquidity osmomath.BigDec `json:"total_amount_of_liquidity"`
}

// Represents the state of a specific price tick in a liquidity pool.
//
// The state is split into two parts for the ask and bid directions.
type OrderbookTickState struct {
	// Values for the ask direction of the tick
	AskValues OrderbookTickValues `json:"ask_values"`
	// Values for the bid direction of the tick
	BidValues OrderbookTickValues `json:"bid_values"`
}

type OrderbookTickIdAndState struct {
	TickId    int64              `json:"tick_id"`
	TickState OrderbookTickState `json:"tick_state"`
}
