package fillbot

import (
	"errors"
	"testing"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/stretchr/testify/require"
)

func swapMsg(sender string, poolIDs ...uint64) *poolmanagertypes.MsgSwapExactAmountIn {
	routes := make([]poolmanagertypes.SwapAmountInRoute, len(poolIDs))
	for i, id := range poolIDs {
		routes[i] = poolmanagertypes.SwapAmountInRoute{PoolId: id}
	}
	return &poolmanagertypes.MsgSwapExactAmountIn{
		Sender: sender,
		Routes: routes,
	}
}

func testMsgCtx(sender string, feeCap string, gas uint64, poolIDs ...uint64) msgContext {
	return msgContext{
		sdkMsg:          swapMsg(sender, poolIDs...),
		maxFeeCap:       osmomath.MustNewDecFromStr(feeCap),
		adjustedGasUsed: gas,
	}
}

func msgSenders(msgs []msgContext) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.sdkMsg.(*poolmanagertypes.MsgSwapExactAmountIn).Sender
	}
	return out
}

func TestTxContextRankAndFilterMsgs(t *testing.T) {
	tests := []struct {
		name            string
		msgs            []msgContext
		expectedSenders []string
		expectedGas     uint64
		expectedFeeCap  string
	}{
		{
			name: "ranks descending by profit and drops overlapping pools",
			msgs: []msgContext{
				testMsgCtx("low-overlap", "5", 100, 1, 3),
				testMsgCtx("high", "10", 200, 1, 2),
				testMsgCtx("mid-disjoint", "8", 150, 3, 4),
			},
			// "high" (pools 1,2) beats "low-overlap" (pool 1), so pool 3 remains free for "mid-disjoint".
			expectedSenders: []string{"high", "mid-disjoint"},
			expectedGas:     350,
			expectedFeeCap:  "18",
		},
		{
			name: "all messages overlap leaves only highest profit",
			msgs: []msgContext{
				testMsgCtx("first", "3", 100, 7),
				testMsgCtx("best", "9", 250, 7, 8),
				testMsgCtx("second", "6", 150, 8),
			},
			expectedSenders: []string{"best"},
			expectedGas:     250,
			expectedFeeCap:  "9",
		},
		{
			name: "non-swap sdk.Msg is skipped",
			msgs: []msgContext{
				{
					sdkMsg:          &banktypes.MsgSend{},
					maxFeeCap:       osmomath.MustNewDecFromStr("100"),
					adjustedGasUsed: 500,
				},
				testMsgCtx("valid", "4", 120, 10),
			},
			expectedSenders: []string{"valid"},
			expectedGas:     120,
			expectedFeeCap:  "4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := newTxContext()
			for _, m := range tc.msgs {
				tx.addMsg(m)
			}

			tx.rankAndFilterMsgs()

			require.Equal(t, tc.expectedSenders, msgSenders(tx.msgs))
			require.Equal(t, tc.expectedGas, tx.adjustedGasUsedTotal)
			require.Equal(t, osmomath.MustNewDecFromStr(tc.expectedFeeCap), tx.maxTxFeeCap)
			require.Len(t, tx.sdkMsgs(), len(tc.expectedSenders))
		})
	}
}

func TestFillWithFallback(t *testing.T) {
	batchErr := errors.New("batch simulation failed")

	tests := []struct {
		name          string
		msgs          []msgContext
		batchFails    bool
		expectedCalls [][]string
		expectedErr   error
	}{
		{
			name: "batch succeeds with no fallback",
			msgs: []msgContext{
				testMsgCtx("a", "10", 100, 1),
				testMsgCtx("b", "8", 100, 2),
			},
			batchFails:    false,
			expectedCalls: [][]string{{"a", "b"}},
		},
		{
			name: "batch fails and falls back only to surviving non-overlapping messages",
			msgs: []msgContext{
				testMsgCtx("dropped-overlap", "5", 100, 1),
				testMsgCtx("high", "10", 200, 1),
				testMsgCtx("disjoint", "8", 150, 2),
			},
			batchFails: true,
			expectedCalls: [][]string{
				{"high", "disjoint"},
				{"high"},
				{"disjoint"},
			},
		},
		{
			name: "batch fails with only one surviving message after filter returns error without retry",
			msgs: []msgContext{
				testMsgCtx("high", "10", 200, 1),
				testMsgCtx("dropped-overlap", "5", 100, 1),
			},
			batchFails:    true,
			expectedCalls: [][]string{{"high"}},
			expectedErr:   batchErr,
		},
		{
			name: "single message batch failure returns error without retry",
			msgs: []msgContext{
				testMsgCtx("only", "10", 200, 1),
			},
			batchFails:    true,
			expectedCalls: [][]string{{"only"}},
			expectedErr:   batchErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &orderbookFillerIngestPlugin{logger: &log.NoOpLogger{}}
			blockCtx := &blockContext{tx: newTxContext()}
			for _, m := range tc.msgs {
				blockCtx.tx.addMsg(m)
			}

			var calls [][]string
			stubTryFill := func(_ *blockContext, txCtx *txContext) error {
				txCtx.rankAndFilterMsgs()
				calls = append(calls, msgSenders(txCtx.msgs))
				if len(calls) == 1 && tc.batchFails {
					return batchErr
				}
				return nil
			}

			err := plugin.fillWithFallback(blockCtx, stubTryFill)
			require.Equal(t, tc.expectedErr, err)
			require.Equal(t, tc.expectedCalls, calls)
		})
	}
}
