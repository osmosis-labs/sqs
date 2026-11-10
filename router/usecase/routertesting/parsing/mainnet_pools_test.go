package parsing_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/v31/ingest/types/json"
	"github.com/osmosis-labs/sqs/domain/mocks"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"

	"github.com/osmosis-labs/osmosis/osmomath"
	concentratedmodel "github.com/osmosis-labs/osmosis/v31/x/concentrated-liquidity/model"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
)

var (
	zeroMinPoolLiquidityCap                   = osmomath.ZeroInt()
	testPoolToMarshal       ingesttypes.PoolI = &mocks.MockRoutablePool{
		ChainPoolModel: &concentratedmodel.Pool{
			Id:                   1,
			Token0:               routertesting.Denom0,
			Token1:               routertesting.Denom1,
			CurrentTickLiquidity: routertesting.DefaultLiquidityAmt,
			CurrentTick:          routertesting.DefaultCurrentTick,
			TickSpacing:          1,
			LastLiquidityUpdate:  time.Unix(1, 1).UTC(),
			SpreadFactor:         routertesting.DefaultSpreadFactor,
			CurrentSqrtPrice:     osmomath.OneBigDec(),
		},
		PoolLiquidityCap: osmomath.OneInt(),
		Balances:         routertesting.DefaultPoolBalances,
		Denoms:           []string{routertesting.Denom0, routertesting.Denom1},
		SpreadFactor:     routertesting.DefaultSpreadFactor,
		PoolType:         poolmanagertypes.Concentrated,
	}

	defaultTickModel = ingesttypes.TickModel{
		Ticks: []ingesttypes.LiquidityDepthsWithRange{
			{
				LiquidityAmount: osmomath.OneDec(),
				LowerTick:       1,
				UpperTick:       2,
			},
		},
		CurrentTickIndex: 0,
		HasNoLiquidity:   false,
	}
)

// This test validates that StorePools stores pools to a file that ReadPools can then read
// back into the system. The file is written to a temporary directory: the file of the same
// name next to this test is the mainnet state that the other tests read.
func TestStoreFilesAndReadBack(t *testing.T) {
	testFileName := t.TempDir() + string(os.PathSeparator) + "pools.json"

	err := parsing.StorePools([]ingesttypes.PoolI{testPoolToMarshal}, map[uint64]*ingesttypes.TickModel{
		testPoolToMarshal.GetId(): &defaultTickModel,
	}, testFileName)
	require.NoError(t, err)

	pools, _, err := parsing.ReadPools(testFileName)
	require.NoError(t, err)

	require.Equal(t, 1, len(pools))
	for _, pool := range pools {
		require.NoError(t, pool.Validate(zeroMinPoolLiquidityCap))
	}
}

// This test validates that unmarshalling and marshalling a pool works as expected.
func TestMarshalUnmarshalPool(t *testing.T) {
	serializedPools, err := parsing.MarshalPool(testPoolToMarshal)
	require.NoError(t, err)

	var interimPools parsing.SerializedPool
	err = json.Unmarshal(serializedPools, &interimPools)
	require.NoError(t, err)

	unmarshalledPool, err := parsing.UnmarshalPool(interimPools)
	require.NoError(t, err)

	require.Equal(t, testPoolToMarshal.GetUnderlyingPool(), unmarshalledPool.GetUnderlyingPool())
	require.Equal(t, testPoolToMarshal.GetSQSPoolModel(), unmarshalledPool.GetSQSPoolModel())
}

// This test validates that unmarshalling and marshalling a taker fee map works as expected.
func TestMarshalUnmarshalTakerFeeMap(t *testing.T) {
	takerFeeMap := ingesttypes.TakerFeeMap{
		ingesttypes.DenomPair{
			Denom0: routertesting.Denom0,
			Denom1: routertesting.Denom1,
		}: osmomath.OneDec(),
	}

	takerFeeMapBz, err := json.Marshal(takerFeeMap)
	require.NoError(t, err)

	unmarshalledTakerFeeMap := ingesttypes.TakerFeeMap{}
	err = json.Unmarshal(takerFeeMapBz, &unmarshalledTakerFeeMap)
	require.NoError(t, err)

	require.Equal(t, takerFeeMap, unmarshalledTakerFeeMap)
}
