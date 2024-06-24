package cosmwasmpool

import (
	"github.com/osmosis-labs/osmosis/osmomath"
)

const (
	ORDERBOOK_CONTRACT_NAME               = "crates.io:sumtree-orderbook"
	ORDERBOOK_MIN_CONTRACT_VERSION        = "0.1.0"
	ORDERBOOK_CONTRACT_VERSION_CONSTRAINT = ">= " + ORDERBOOK_MIN_CONTRACT_VERSION
)

func (model *CosmWasmPoolModel) IsOrderbook() bool {
	return model.ContractInfo.Matches(
		ORDERBOOK_CONTRACT_NAME,
		mustParseSemverConstraint(ORDERBOOK_CONTRACT_VERSION_CONSTRAINT),
	)
}

// OrderbookData, since v1.0.0
type OrderbookData struct {
	QuoteDenom  string          `json:"quote_denom"`
	BaseDenom   string          `json:"base_denom"`
	NextBidTick int64           `json:"next_bid_tick"`
	NextAskTick int64           `json:"next_ask_tick"`
	Ticks       []OrderbookTick `json:"ticks"`
}

// Represents Total Amount of Liquidity at tick (TAL) of a specific price tick in a liquidity pool.
// - Every limit order placement increments this value.
// - Every swap at this tick decrements this value.
// - Every cancellation decrements this value.
//
// It is split into two parts for the ask and bid directions.
type OrderbookTickLiquidity struct {
	// Total Amount of Liquidity at tick (TAL) for the bid direction of the tick
	BidLiquidity osmomath.BigDec `json:"bid_liquidity"`
	// Total Amount of Liquidity at tick (TAL) for the ask direction of the tick
	AskLiquidity osmomath.BigDec `json:"ask_liquidity"`
}

type OrderbookTick struct {
	TickId        int64                  `json:"tick_id"`
	TickLiquidity OrderbookTickLiquidity `json:"tick_liquidity"`
}
