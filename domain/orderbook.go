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

func (m *OrderbookTickModel) GetTickIndexById(tickId int64) int {
	for i, tick := range m.TickStates {
		if tick.TickId == tickId {
			return i
		}
	}
	return -1
}

type OrderbookTickState struct {
	TickId    int64
	AskValues OrderbookTickValues
	BidValues OrderbookTickValues
}

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
	TotalAmountOfLiquidity      osmomath.BigDec "json:\"total_amount_of_liquidity\""
	CumulativeTotalValue        osmomath.BigDec "json:\"cumulative_total_value\""
	EffectiveTotalAmountSwapped osmomath.BigDec "json:\"effective_total_amount_swapped\""
	CumulativeRealizedCancels   osmomath.BigDec "json:\"cumulative_realized_cancels\""
}

func (t *OrderbookTickValues) GetFillableAmount(input osmomath.BigDec) osmomath.BigDec {
	if input.LT(t.TotalAmountOfLiquidity) {
		return input
	}
	return t.TotalAmountOfLiquidity
}
