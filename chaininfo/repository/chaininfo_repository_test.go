package chaininforepo_test

import (
	"testing"

	chaininforepo "github.com/osmosis-labs/sqs/chaininfo/repository"
	"github.com/stretchr/testify/require"
)

// TestStoreAndGetLatestHeight tests storing the latest blockchain height
func TestStoreAndGetLatestHeight(t *testing.T) {
	repo := chaininforepo.New()
	height := uint64(100)

	repo.StoreLatestHeight(height)

	require.Equal(t, height, repo.GetLatestHeight())

	// change height
	height = 200
	repo.StoreLatestHeight(height)

	require.Equal(t, height, repo.GetLatestHeight())
}
