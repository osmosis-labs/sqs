package pools_test

import (
	"encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
)

// The quote response lists result pools. code_id identifies the contract of a CosmWasm pool;
// other pools have no code ID, and a zero would read as a real one, so it is omitted.
func TestRoutableResultPoolJSON(t *testing.T) {
	takerFee := osmomath.MustNewDecFromStr("0.001")
	spreadFactor := osmomath.MustNewDecFromStr("0.002")

	t.Run("non-CosmWasm pool omits code_id and encodes nil balances as []", func(t *testing.T) {
		pool := pools.NewRoutableResultPool(1, poolmanagertypes.Balancer, spreadFactor, "uosmo", takerFee, osmomath.ZeroInt(), 0)
		bz, err := json.Marshal(pool)
		require.NoError(t, err)
		require.JSONEq(t, `{
			"id": 1,
			"type": 0,
			"balances": [],
			"spread_factor": "0.002000000000000000",
			"token_out_denom": "uosmo",
			"taker_fee": "0.001000000000000000",
			"liquidity_cap": "0"
		}`, string(bz))
	})

	t.Run("CosmWasm pool keeps code_id", func(t *testing.T) {
		pool := pools.NewRoutableResultPool(2, poolmanagertypes.CosmWasm, spreadFactor, "uosmo", takerFee, osmomath.NewInt(10), 814)
		bz, err := json.Marshal(pool)
		require.NoError(t, err)
		require.JSONEq(t, `{
			"id": 2,
			"type": 3,
			"balances": [],
			"spread_factor": "0.002000000000000000",
			"token_out_denom": "uosmo",
			"taker_fee": "0.001000000000000000",
			"liquidity_cap": "10",
			"code_id": 814
		}`, string(bz))
	})
}
