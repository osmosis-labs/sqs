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

type OrderbookDirection int

const (
	BID OrderbookDirection = 1
	ASK OrderbookDirection = -1
)

func (d *OrderbookDirection) String() string {
	switch *d {
	case BID:
		return "BID"
	case ASK:
		return "ASK"
	default:
		return "UNKNOWN"
	}
}

func (d *OrderbookDirection) Opposite() OrderbookDirection {
	switch *d {
	case BID:
		return ASK
	case ASK:
		return BID
	default:
		return 0
	}
}

// OrderbookData, since v1.0.0
type OrderbookData struct {
	QuoteDenom  string                    `json:"quote_denom"`
	BaseDenom   string                    `json:"base_denom"`
	NextBidTick int64                     `json:"next_bid_tick"`
	NextAskTick int64                     `json:"next_ask_tick"`
	Ticks       []OrderbookTickIdAndState `json:"ticks"`
}

// Returns tick state index for the given ID
func (d *OrderbookData) GetTickIndexById(tickId int64) int {
	for i, tick := range d.Ticks {
		if tick.TickId == tickId {
			return i
		}
	}
	return -1
}

type OrderbookTickValues struct {
	// Total Amount of Liquidity at tick (TAL)
	// - Every limit order placement increments this value.
	// - Every swap at this tick decrements this value.
	// - Every cancellation decrements this value.
	TotalAmountOfLiquidity osmomath.BigDec `json:"total_amount_of_liquidity"`
}

// Determines how much of a given amount can be filled by the current tick state (independent for each direction)
func (t *OrderbookTickValues) GetFillableAmount(input osmomath.BigDec) osmomath.BigDec {
	if input.LT(t.TotalAmountOfLiquidity) {
		return input
	}
	return t.TotalAmountOfLiquidity
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

// Returns the related values for a given direction on the current tick
func (s *OrderbookTickState) GetTickValues(direction OrderbookDirection) (OrderbookTickValues, error) {
	switch direction {
	case ASK:
		return s.AskValues, nil
	case BID:
		return s.BidValues, nil
	default:
		return OrderbookTickValues{}, OrderbookPoolInvalidDirectionError{Direction: direction}
	}
}

type OrderbookTickIdAndState struct {
	TickId    int64              `json:"tick_id"`
	TickState OrderbookTickState `json:"tick_state"`
}
