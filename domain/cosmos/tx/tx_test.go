package tx_test

import (
	"testing"

	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v31/app"

	sdk "github.com/cosmos/cosmos-sdk/types"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	"github.com/stretchr/testify/assert"
)

var (
	encodingConfig = app.MakeEncodingConfig()

	newMsg = func(sender, contract, msg string) sdk.Msg {
		return &wasmtypes.MsgExecuteContract{
			Sender:   sender,
			Contract: contract,
			Msg:      []byte(msg),
			Funds:    sdk.NewCoins(),
		}
	}
)

func TestCalculateFeeAmount(t *testing.T) {
	tests := []struct {
		name     string
		baseFee  osmomath.Dec
		gas      uint64
		expected osmomath.Int
	}{
		{
			name:     "Zero base fee",
			baseFee:  osmomath.NewDec(0),
			gas:      1000,
			expected: osmomath.NewInt(0),
		},
		{
			name:     "Zero gas",
			baseFee:  osmomath.NewDec(100),
			gas:      0,
			expected: osmomath.NewInt(0),
		},
		{
			name:     "Normal case",
			baseFee:  osmomath.NewDecWithPrec(5, 1), // 0.5
			gas:      100000,
			expected: osmomath.NewInt(50000),
		},
		{
			name:     "Large numbers",
			baseFee:  osmomath.NewDec(1000),
			gas:      1000000,
			expected: osmomath.NewInt(1000000000),
		},
		{
			name:     "Fractional result",
			baseFee:  osmomath.NewDecWithPrec(33, 2), // 0.33
			gas:      10000,
			expected: osmomath.NewInt(3300),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sqstx.CalculateFeeAmount(tt.baseFee, tt.gas)
			assert.True(t, tt.expected.Equal(result), "Expected %s, but got %s", tt.expected, result)
		})
	}
}
