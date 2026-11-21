package usecase

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	chaininforepo "github.com/osmosis-labs/sqs/chaininfo/repository"
)

// TestGetLatestHeightStale checks that an unchanged height stays healthy inside the window
// however often it is polled, and goes stale once the window has passed since it last increased.
func TestGetLatestHeightStale(t *testing.T) {
	repo := chaininforepo.New()
	uc := NewChainInfoUsecase(repo, nil)

	clock := time.Unix(1_000_000, 0)
	uc.now = func() time.Time { return clock }

	repo.StoreLatestHeight(100)
	height, err := uc.GetLatestHeight()
	require.NoError(t, err)
	require.Equal(t, uint64(100), height)

	// Polling every 10s with no new height stays healthy until the window passes.
	for elapsed := 10; elapsed <= MaxAllowedHeightUpdateTimeDeltaSecs; elapsed += 10 {
		clock = clock.Add(10 * time.Second)
		_, err = uc.GetLatestHeight()
		require.NoError(t, err)
	}

	clock = clock.Add(10 * time.Second)
	_, err = uc.GetLatestHeight()
	require.Error(t, err)

	// A new height recovers it.
	repo.StoreLatestHeight(101)
	_, err = uc.GetLatestHeight()
	require.NoError(t, err)
}

// TestValidateOrderbookTicksGracePeriod checks that orderbooks missing ticks fail validation only once
// the grace period has passed, and that recovering clears it, using the injected clock.
func TestValidateOrderbookTicksGracePeriod(t *testing.T) {
	uc := NewChainInfoUsecase(chaininforepo.New(), nil)

	clock := time.Unix(1_000_000, 0)
	uc.now = func() time.Time { return clock }

	require.NoError(t, uc.ValidateOrderbookTicks())

	// More than the allowed share of orderbooks without ticks starts the grace period.
	uc.SetOrderbooksWithoutTicks(10, 10)
	require.NoError(t, uc.ValidateOrderbookTicks())

	clock = clock.Add(orderbookTicksGracePeriod)
	require.NoError(t, uc.ValidateOrderbookTicks())

	// A later report does not restart the period.
	uc.SetOrderbooksWithoutTicks(9, 10)
	clock = clock.Add(time.Second)
	require.Error(t, uc.ValidateOrderbookTicks())

	// Recovery clears it, and a new failure gets a new grace period.
	uc.SetOrderbooksWithoutTicks(0, 10)
	require.NoError(t, uc.ValidateOrderbookTicks())
	uc.SetOrderbooksWithoutTicks(10, 10)
	clock = clock.Add(orderbookTicksGracePeriod)
	require.NoError(t, uc.ValidateOrderbookTicks())
}
