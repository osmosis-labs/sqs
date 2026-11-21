package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
)

// TestUpdateHeightsDoNotGoBack checks that a price or pool liquidity update for an
// older height, finishing after a newer one, does not move the health height back.
func TestUpdateHeightsDoNotGoBack(t *testing.T) {
	p := NewChainInfoUsecase(nil, nil)

	require.NoError(t, p.OnPricingUpdate(context.Background(), 10, domain.BlockPoolMetadata{}, nil, ""))
	require.NoError(t, p.OnPricingUpdate(context.Background(), 7, domain.BlockPoolMetadata{}, nil, ""))
	require.Equal(t, uint64(10), p.latestPricesUpdateHeight)

	require.NoError(t, p.OnPoolLiquidityCompute(context.Background(), 10, domain.BlockPoolMetadata{}))
	require.NoError(t, p.OnPoolLiquidityCompute(context.Background(), 7, domain.BlockPoolMetadata{}))
	require.Equal(t, uint64(10), p.latestPoolLiquidityUpdateHeight)

	require.NoError(t, p.OnPricingUpdate(context.Background(), 11, domain.BlockPoolMetadata{}, nil, ""))
	require.Equal(t, uint64(11), p.latestPricesUpdateHeight)
}
