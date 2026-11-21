package fillbot

import (
	"context"
	"sort"
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"
)

var defaultMinArbGasFee = osmomath.MustNewBigDecFromStr("0.1")

type blockGasPrice struct {
	gasPrice                  osmomath.Dec
	gasPriceDefaultQuoteDenom osmomath.BigDec
}

type blockContext struct {
	ctx          context.Context
	gasPrice     blockGasPrice
	userBalances sdk.Coins
	tx           *txContext
	prices       domain.PricesResult
	blockHeight  uint64
}

func newBlockContext(ctx context.Context, orderBookDenomPrices domain.PricesResult, userBalances sdk.Coins, defaultQuoteDenom string, blockHeight uint64) *blockContext {
	baseDenomPrice := orderBookDenomPrices.GetPriceForDenom(orderbookplugindomain.BaseDenom, defaultQuoteDenom)
	return &blockContext{
		ctx:          ctx,
		tx:           newTxContext(),
		blockHeight:  blockHeight,
		userBalances: userBalances,
		prices:       orderBookDenomPrices,
		gasPrice: blockGasPrice{
			gasPrice:                  defaultMinArbGasFee.Dec(),
			gasPriceDefaultQuoteDenom: baseDenomPrice.Mul(defaultMinArbGasFee),
		},
	}
}

type msgContext struct {
	adjustedGasUsed uint64
	sdkMsg          sdk.Msg
	maxFeeCap       osmomath.Dec
}

type txContext struct {
	msgsMx               sync.Mutex
	msgs                 []msgContext
	adjustedGasUsedTotal uint64
	maxTxFeeCap          osmomath.Dec
}

func newTxContext() *txContext {
	return &txContext{
		maxTxFeeCap: osmomath.ZeroDec(),
	}
}

func (t *txContext) addMsg(msg msgContext) {
	t.msgsMx.Lock()
	defer t.msgsMx.Unlock()

	t.msgs = append(t.msgs, msg)
	t.adjustedGasUsedTotal += msg.adjustedGasUsed
	t.maxTxFeeCap = t.maxTxFeeCap.Add(msg.maxFeeCap)
}

func (t *txContext) sdkMsgs() []sdk.Msg {
	msgs := make([]sdk.Msg, len(t.msgs))
	for i, msg := range t.msgs {
		msgs[i] = msg.sdkMsg
	}
	return msgs
}

// rankAndFilterMsgs sorts messages by descending expected profit and drops any
// message that shares a pool with a higher-ranked message so one arb cannot invalidate another.
func (t *txContext) rankAndFilterMsgs() {
	sort.Slice(t.msgs, func(i, j int) bool {
		return t.msgs[i].maxFeeCap.GT(t.msgs[j].maxFeeCap)
	})

	uniquePools := make(map[uint64]struct{})
	var filtered []msgContext
	var gasTotal uint64
	feeCap := osmomath.ZeroDec()

	for _, msgCtx := range t.msgs {
		msg, ok := msgCtx.sdkMsg.(*poolmanagertypes.MsgSwapExactAmountIn)
		if !ok {
			continue
		}

		overlaps := false
		for _, route := range msg.Routes {
			if _, exists := uniquePools[route.PoolId]; exists {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}

		for _, route := range msg.Routes {
			uniquePools[route.PoolId] = struct{}{}
		}

		filtered = append(filtered, msgCtx)
		gasTotal += msgCtx.adjustedGasUsed
		feeCap = feeCap.Add(msgCtx.maxFeeCap)
	}

	t.msgs = filtered
	t.adjustedGasUsedTotal = gasTotal
	t.maxTxFeeCap = feeCap
}
