package passthroughdomain

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	concentratedLiquidity "github.com/osmosis-labs/osmosis/v31/x/concentrated-liquidity/client/queryproto"
)

// nilUserPositionsClient answers UserPositions with neither a response nor an error, which a
// proxy or a misbehaving node can do. Every other method is left unimplemented.
type nilUserPositionsClient struct {
	concentratedLiquidity.QueryClient
}

func (nilUserPositionsClient) UserPositions(context.Context, *concentratedLiquidity.UserPositionsRequest, ...grpc.CallOption) (*concentratedLiquidity.UserPositionsResponse, error) {
	return nil, nil
}

// Validates that a nil UserPositions response counts as an empty last page instead of panicking.
func TestUserPositionsBalances_NilResponse(t *testing.T) {
	client := &passthroughGRPCClient{concentratedLiquidityQueryClient: nilUserPositionsClient{}}

	pooledCoins, rewardCoins, err := client.UserPositionsBalances(context.Background(), "osmo1address")
	require.NoError(t, err)
	require.True(t, pooledCoins.IsZero())
	require.True(t, rewardCoins.IsZero())
}
