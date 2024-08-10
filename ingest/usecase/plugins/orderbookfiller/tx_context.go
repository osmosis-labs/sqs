package orderbookfiller

import (
	"sort"
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

type txContext struct {
	msgsMx               sync.Mutex
	msgs                 []msgContext
	adjustedGasUsedTotal uint64

	maxTxFeeCap sdk.Dec
}

func newTxContext() *txContext {
	return &txContext{
		maxTxFeeCap: osmomath.ZeroDec(),
	}
}

func (t *txContext) AddMsg(msg msgContext) {
	t.msgsMx.Lock()
	defer t.msgsMx.Unlock()

	t.msgs = append(t.msgs, msg)

	t.adjustedGasUsedTotal += msg.adjustedGasUsed

	t.maxTxFeeCap.AddMut(msg.maxTxFeeCap)
}

func (t *txContext) UpdateAdjustedGasTotal(newGasUsed uint64) {
	t.adjustedGasUsedTotal = newGasUsed
}

func (t *txContext) rankAndFilterPools() {
	uniquePools := make(map[uint64]struct{})

	// Sort the messages by expected profit
	sort.Slice(t.msgs, func(i, j int) bool {
		// In the context of arbs, this is max profit
		return t.msgs[i].maxTxFeeCap.GT(t.msgs[j].maxTxFeeCap)
	})

	finalTxContext := newTxContext()

	for _, msgCtx := range t.msgs {
		if msg, ok := msgCtx.msg.(*poolmanagertypes.MsgSwapExactAmountIn); ok {
			shouldSkipMessage := false

			uniquePoolsInMsg := make(map[uint64]struct{})

			for _, route := range msg.Routes {
				// Avoid overlapping pools in the transaction
				// As the same pool in one arb might invalidate the other arb
				// We first sort the messages by profit, so we can safely skip the pool
				if _, ok := uniquePools[route.PoolId]; ok {
					shouldSkipMessage = true
					break
				}

				// Add the pool to the unique pools in the message
				uniquePoolsInMsg[route.PoolId] = struct{}{}
			}

			if shouldSkipMessage {
				continue
			}

			// If we did not skip the message, add the pools to the unique pools
			for poolID := range uniquePoolsInMsg {
				uniquePools[poolID] = struct{}{}
			}

			// Keep the message in the transaction context
			finalTxContext.AddMsg(msgCtx)
		}
	}

	// Update the transaction context
	t.adjustedGasUsedTotal = finalTxContext.adjustedGasUsedTotal
	t.maxTxFeeCap = finalTxContext.maxTxFeeCap
	t.msgs = finalTxContext.msgs
}
