package domain_test

import (
	"encoding/json/v2"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

// adjusted_gas_used is present only when the swap was simulated. A failed or skipped simulation
// leaves it zero, which would read as "costs no gas", so it is omitted instead.
func TestTxFeeInfoJSON(t *testing.T) {
	tests := []struct {
		name string
		info domain.TxFeeInfo
		want string
	}{
		{
			name: "simulated",
			info: domain.TxFeeInfo{AdjustedGasUsed: 1_000_000, FeeCoin: sdk.NewCoin("uosmo", osmomath.NewInt(5000)), BaseFee: osmomath.MustNewDecFromStr("0.005")},
			want: `{"adjusted_gas_used":1000000,"fee_coin":{"denom":"uosmo","amount":"5000"},"base_fee":"0.005000000000000000"}`,
		},
		{
			name: "base fee only",
			info: domain.TxFeeInfo{BaseFee: osmomath.MustNewDecFromStr("0.5")},
			want: `{"fee_coin":{"amount":"0"},"base_fee":"0.500000000000000000"}`,
		},
		{
			name: "simulation failed",
			info: domain.TxFeeInfo{Err: "out of gas", BaseFee: osmomath.MustNewDecFromStr("0.5")},
			want: `{"fee_coin":{"amount":"0"},"base_fee":"0.500000000000000000","error":"out of gas"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bz, err := json.Marshal(tt.info)
			require.NoError(t, err)
			require.JSONEq(t, tt.want, string(bz))
		})
	}
}
