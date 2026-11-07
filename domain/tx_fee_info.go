package domain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// TxFeeInfo represents the fee information for a transaction
type TxFeeInfo struct {
	// AdjustedGasUsed is set only when the swap was simulated; zero means "not simulated" and is omitted.
	AdjustedGasUsed uint64       `json:"adjusted_gas_used,omitzero"`
	FeeCoin         sdk.Coin     `json:"fee_coin"`
	BaseFee         osmomath.Dec `json:"base_fee"`
	Err             string       `json:"error,omitempty"`
}
