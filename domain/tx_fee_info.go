package domain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// TxFeeInfo represents the fee information for a transaction
type TxFeeInfo struct {
	AdjustedGasUsed uint64       `json:"adjusted_gas_used,omitempty"`
	FeeCoin         sdk.Coin     `json:"fee_coin,omitempty"`
	BaseFee         osmomath.Dec `json:"base_fee"`
	Err             string       `json:"error,omitempty"`
}
