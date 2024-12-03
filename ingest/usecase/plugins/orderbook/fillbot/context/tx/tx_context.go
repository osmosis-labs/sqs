package txctx

import (
	"sort"
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
	msgctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/fillbot/context/msg"
)

// TxContextI is an interface responsible for abstracting transaction context
// It contains all messages to be executed as a single transaction.
// Total adjusted gas used after simulating the transaction and a max transaction
// fee capitalization. The latter acts as a guardrail against the fee making transaction
// execution be unprofitable.
type TxContextI interface {
	// GetSDKMsgs returns chain messages associated with this transaction context as sdk.Msg.
	GetSDKMsgs() []sdk.Msg

	// GetMsgs returns message contexts
	GetMsgs() []msgctx.MsgContextI

	// GetAdjustedGasUsedTotal returns the gas used after simulating this transaction
	// against chain and adjusting the gas by a constant pre-configured multiplier.
	GetAdjustedGasUsedTotal() uint64

	// GetMaxTxFeeCap returns the max tx fee capitalization allowes to fully execute this transaction.
	// For example, in the context of cyclic arbs, we only execute the transaction it is smaller than expected profit.
	GetMaxTxFeeCap() osmomath.Dec

	// RankAndFilterMsgs ranks and filters messages so that one message cannot invalidate the other.
	// For the cyclic arbitrage tx, it ensures that there are no 2 messages that execute over the same pool.
	// This is to avoid one message invalidate the arb for the other. Instead, we rank the messages by expected
	// profit, keep the top one and discard the rest.
	RankAndFilterMsgs()

	// UpdateAdjustedGasTotal updates the adjusted gas total upon resimulating the transaction against chain.
	UpdateAdjustedGasTotal(adjustedGasTotal uint64)

	// AddMsg adds the given message to the transaction context.
	// Increases the total adjusted gas used and max tx fee cap with the message values.
	AddMsg(msgCtx msgctx.MsgContextI)
}

// txContext is a concrete implementation of TxContextI.
type txContext struct {
	msgsMx               sync.Mutex
	msgs                 []msgctx.MsgContextI
	adjustedGasUsedTotal uint64
	maxTxFeeCap          osmomath.Dec
}

// GetSDKMsgs implements TxContextI.
func (t *txContext) GetSDKMsgs() []sdk.Msg {
	msgs := make([]sdk.Msg, len(t.msgs))
	for i, msg := range t.msgs {
		msgs[i] = msg.AsSDKMsg()
	}

	return msgs
}

var _ TxContextI = &txContext{}

// New returns the new transaction context
func New() *txContext {
	return &txContext{
		maxTxFeeCap: osmomath.ZeroDec(),
	}
}

// GetAdjustedGasUsedTotal implements TxContextI.
func (t *txContext) GetAdjustedGasUsedTotal() uint64 {
	return t.adjustedGasUsedTotal
}

// AddMsg implements TxContextI.
func (t *txContext) AddMsg(msg msgctx.MsgContextI) {
	t.msgsMx.Lock()
	defer t.msgsMx.Unlock()

	t.msgs = append(t.msgs, msg)

	t.adjustedGasUsedTotal += msg.GetAdjustedGasUsed()

	t.maxTxFeeCap = t.maxTxFeeCap.Add(msg.GetMaxFeeCap())
}

// UpdateAdjustedGasTotal implements TxContextI.
func (t *txContext) UpdateAdjustedGasTotal(newGasUsed uint64) {
	t.adjustedGasUsedTotal = newGasUsed
}

// RankAndFilterMsgs implements TxContextI.
func (t *txContext) RankAndFilterMsgs() {
	uniquePools := make(map[uint64]struct{})

	// Sort the messages by expected profit
	sort.Slice(t.msgs, func(i, j int) bool {
		// In the context of arbs, this is max profit
		return t.msgs[i].GetMaxFeeCap().GT(t.msgs[j].GetMaxFeeCap())
	})

	finalTxContext := New()

	for _, msgCtx := range t.msgs {
		if msg, ok := msgCtx.AsSDKMsg().(*poolmanagertypes.MsgSwapExactAmountIn); ok {
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

// GetMaxTxFeeCap implements TxContextI.
func (t *txContext) GetMaxTxFeeCap() osmomath.Dec {
	return t.maxTxFeeCap
}

// GetMsgs implements TxContextI.
func (t *txContext) GetMsgs() []msgctx.MsgContextI {
	return t.msgs
}
