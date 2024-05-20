package domain

import (
	"github.com/osmosis-labs/osmosis/osmomath"
)

const (
	BID = 1
	ASK = -1
)

type OrderbookTickModel struct {
	NextAskTickId int64
	NextBidTickId int64
	TickStates    []OrderbookTickState
}

// Returns tick state index for the given ID
func (m *OrderbookTickModel) GetTickIndexById(tickId int64) int {
	for i, tick := range m.TickStates {
		if tick.TickId == tickId {
			return i
		}
	}
	return -1
}

type OrderbookTickState struct {
	// The ID of the tick
	TickId int64
	// All related values for the Ask direction for current tick
	AskValues OrderbookTickValues
	// All related values for the Bid direction for current tick
	BidValues OrderbookTickValues
}

// Returns the related values for a given direction on the current tick
func (s *OrderbookTickState) GetTickValues(direction int64) (OrderbookTickValues, error) {
	if direction == -1 {
		return s.AskValues, nil
	} else if direction == 1 {
		return s.BidValues, nil
	} else {
		return OrderbookTickValues{}, OrderbookPoolInvalidDirectionError{Direction: direction}
	}
}

type OrderbookTickValues struct {
	// The total amount of available liquidity on the current tick
	TotalAmountOfLiquidity osmomath.BigDec "json:\"total_amount_of_liquidity\""
	// The total amount of liquidity placed on the tick
	CumulativeTotalValue osmomath.BigDec "json:\"cumulative_total_value\""
	// The total amount of token swapped on the tick
	EffectiveTotalAmountSwapped osmomath.BigDec "json:\"effective_total_amount_swapped\""
	// The total amount of liquidity cancelled on the tick
	CumulativeRealizedCancels osmomath.BigDec "json:\"cumulative_realized_cancels\""
}

// Determines how much of a given amount can be filled by the current tick state (independent for each direction)
func (t *OrderbookTickValues) GetFillableAmount(input osmomath.BigDec) osmomath.BigDec {
	if input.LT(t.TotalAmountOfLiquidity) {
		return input
	}
	return t.TotalAmountOfLiquidity
}
