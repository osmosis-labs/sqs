package rustffi

import (
	"errors"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/stretchr/testify/require"
)

func TestCompressedMovingAverage(t *testing.T) {
	t.Run("no divisions", func(t *testing.T) {
		divisions, err := NewFFIDivisions([]struct {
			StartedAt   uint64
			UpdatedAt   uint64
			LatestValue osmomath.Dec
			PrevValue   osmomath.Dec
		}{})
		require.NoError(t, err)

		average, err := CompressedMovingAverage(nil, divisions, 100, 1000, 1270)
		require.ErrorContains(t, err, "Missing data points to calculate moving average")
		require.Equal(t, osmomath.Dec{}, average)
	})

	t.Run("2 divisions", func(t *testing.T) {
		// Create a slice of FFIDivisions
		divisions, err := NewFFIDivisions([]struct {
			StartedAt   uint64
			UpdatedAt   uint64
			LatestValue osmomath.Dec
			PrevValue   osmomath.Dec
		}{
			{1100, 1110, osmomath.NewDecWithPrec(20, 2), osmomath.NewDecWithPrec(10, 2)},
			{1200, 1260, osmomath.NewDecWithPrec(30, 2), osmomath.NewDecWithPrec(20, 2)},
		})
		require.NoError(t, err)

		// Call CompressedMovingAverage
		result, err := CompressedMovingAverage(nil, divisions, 100, 1000, 1270)

		// Check for errors
		require.NoError(t, err)

		// Calculate expected result
		expected := osmomath.NewDecWithPrec(10, 2).MulInt64(10).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(90)).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(60)).
			Add(osmomath.NewDecWithPrec(30, 2).MulInt64(10)).
			Quo(osmomath.NewDec(170))

		// Verify the result matches the expected value
		require.Equal(t, expected, result)
	})

	t.Run("test average when div is skipping", func(t *testing.T) {
		// skipping 1 division
		divisionSize := uint64(200)
		windowSize := uint64(600)

		divisions, err := NewFFIDivisions([]struct {
			StartedAt   uint64
			UpdatedAt   uint64
			LatestValue osmomath.Dec
			PrevValue   osmomath.Dec
		}{
			{1100, 1110, osmomath.NewDecWithPrec(20, 2), osmomath.NewDecWithPrec(10, 2)},
			// -- skip 1300 -> 1500 --
			// 20% * 200 - 1 div size
			{1500, 1540, osmomath.NewDecWithPrec(30, 2), osmomath.NewDecWithPrec(20, 2)},
		})
		require.NoError(t, err)

		blockTime := uint64(1600)

		average, err := CompressedMovingAverage(nil, divisions, divisionSize, windowSize, blockTime)
		require.NoError(t, err)

		expected := osmomath.NewDecWithPrec(10, 2).MulInt64(10).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(190)).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(200)). // skipped div
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(40)).
			Add(osmomath.NewDecWithPrec(30, 2).MulInt64(60)).
			Quo(osmomath.NewDec(500))

		require.Equal(t, expected, average)

		latestRemovedDivision, err := NewFFIDivision(700, 750, osmomath.NewDecWithPrec(10, 2), osmomath.NewDecWithPrec(15, 2))
		require.NoError(t, err)

		average, err = CompressedMovingAverage(&latestRemovedDivision, divisions, divisionSize, windowSize, blockTime)
		require.NoError(t, err)

		expected = osmomath.NewDecWithPrec(10, 2).MulInt64(100). // before first div
										Add(osmomath.NewDecWithPrec(10, 2).MulInt64(10)).
										Add(osmomath.NewDecWithPrec(20, 2).MulInt64(190)).
										Add(osmomath.NewDecWithPrec(20, 2).MulInt64(200)). // skipped div
										Add(osmomath.NewDecWithPrec(20, 2).MulInt64(40)).
										Add(osmomath.NewDecWithPrec(30, 2).MulInt64(60)).
										Quo(osmomath.NewDec(600)).
										Sub(osmomath.NewDecWithPrec(1, osmomath.DecPrecision)) // remove round up

		require.Equal(t, expected, average)

		blockTime = uint64(1700)
		average, err = CompressedMovingAverage(&latestRemovedDivision, divisions, divisionSize, windowSize, blockTime)
		require.NoError(t, err)

		expected = osmomath.NewDecWithPrec(10, 2).MulInt64(10).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(190)).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(200)). // skipped div
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(40)).
			Add(osmomath.NewDecWithPrec(30, 2).MulInt64(160)).
			Quo(osmomath.NewDec(600))

		require.Equal(t, expected, average)

		// skipping 2 divisions
		divisionSize = uint64(100)
		windowSize = uint64(600)

		divisions, err = NewFFIDivisions([]struct {
			StartedAt   uint64
			UpdatedAt   uint64
			LatestValue osmomath.Dec
			PrevValue   osmomath.Dec
		}{
			{1100, 1110, osmomath.NewDecWithPrec(20, 2), osmomath.NewDecWithPrec(10, 2)},
			// -- skip 1300 -> 1500 --
			// 20% * 200 - 2 div size
			{1500, 1540, osmomath.NewDecWithPrec(30, 2), osmomath.NewDecWithPrec(20, 2)},
		})
		require.NoError(t, err)

		blockTime = uint64(1600)

		average, err = CompressedMovingAverage(nil, divisions, divisionSize, windowSize, blockTime)
		require.NoError(t, err)

		expected = osmomath.NewDecWithPrec(10, 2).MulInt64(10).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(190)).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(100)). // skipped div
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(100)). // skipped div
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(40)).
			Add(osmomath.NewDecWithPrec(30, 2).MulInt64(60)).
			Quo(osmomath.NewDec(500))

		require.Equal(t, expected, average)

		blockTime = uint64(1710)

		average, err = CompressedMovingAverage(nil, divisions, divisionSize, windowSize, blockTime)
		require.NoError(t, err)

		expected = osmomath.NewDecWithPrec(20, 2).MulInt64(190).
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(100)). // skipped div
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(100)). // skipped div
			Add(osmomath.NewDecWithPrec(20, 2).MulInt64(40)).
			Add(osmomath.NewDecWithPrec(30, 2).MulInt64(170)).
			Quo(osmomath.NewDec(600))

		require.Equal(t, expected, average)
	})
}

func TestIsDivisionOutdated(t *testing.T) {
	division, err := NewFFIDivisionRaw(
		1000000000,
		1000000022,
		osmomath.NewDecWithPrec(10, 2),
		osmomath.NewDecWithPrec(22, 2),
	)
	require.NoError(t, err)

	windowSize := uint64(1000)
	divisionSize := uint64(100)

	testCases := []struct {
		name          string
		blockTime     uint64
		expected      bool
		expectedError error
	}{
		{name: "within window - start", blockTime: 1000000000, expected: false},
		{name: "within window - near end", blockTime: 1000000999, expected: false},
		{name: "within window - at end", blockTime: 1000001000, expected: false},
		{name: "within window - last valid", blockTime: 1000001099, expected: false},
		{name: "out of window - first invalid", blockTime: 1000001100, expected: true},
		{name: "out of window - just after", blockTime: 1000001101, expected: true},
		{name: "out of window - far after", blockTime: 1000001200, expected: true},
		{name: "blocktime too old", blockTime: 1, expectedError: errors.New("Cannot Sub with 1 and 1000")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := IsDivisionOutdated(division, tc.blockTime, windowSize, divisionSize)
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}
