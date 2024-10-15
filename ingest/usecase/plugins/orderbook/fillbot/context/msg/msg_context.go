package msgctx

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// MsgContextI specifies an interface responsible for managing an individual
// chain message.
// It abstracts simulated gas used as if this message was executed via a single
// transaction, max transactionfee capitalizaion for executing this message.
// It acts a safeguard against cases where a high-enough transacion fee may make
// executing the message unprofitable.
// It also abstracts the chain message type itself.
type MsgContextI interface {
	// GetMaxFeeCap returns the max fee capitalization of execution
	// this message as an individual transaction.
	// The fee capitalization is computed and set upon estimating the message
	// as a tx against chain.
	GetMaxFeeCap() osmomath.Dec

	// AsSDKMsg returns the sdk message associated with the context.
	AsSDKMsg() sdk.Msg

	// GetAdjustedGasUsed returns the gas used after simulating this message as a single tx
	// and adjusting the gas by a constant pre-configured multiplier.
	GetAdjustedGasUsed() uint64
}

type msgContext struct {
	adjustedGasUsed uint64
	sdkMSg          sdk.Msg

	maxFeeCap osmomath.Dec
}

// New returns the new message context
func New(maxFeeCap osmomath.Dec, adjustedGasUsed uint64, sdkMsg sdk.Msg) *msgContext {
	return &msgContext{
		maxFeeCap:       maxFeeCap,
		adjustedGasUsed: adjustedGasUsed,
		sdkMSg:          sdkMsg,
	}
}

// GetAdjustedGasUsed implements MsgContextI.
func (m msgContext) GetAdjustedGasUsed() uint64 {
	return m.adjustedGasUsed
}

// AsSDKMsg implements MsgContextI.
func (m msgContext) AsSDKMsg() sdk.Msg {
	return m.sdkMSg
}

// GetMaxFeeCap implements MsgContextI.
func (m msgContext) GetMaxFeeCap() math.LegacyDec {
	return m.maxFeeCap
}

var _ MsgContextI = msgContext{}
