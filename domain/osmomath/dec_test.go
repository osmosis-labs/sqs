package osmomath

import (
	"testing"

	cosmosmath "github.com/osmosis-labs/osmosis/osmomath"
	"github.com/stretchr/testify/require"
)

var testCases = []struct {
	name  string
	input []cosmosmath.BigDec
	min   cosmosmath.BigDec
	max   cosmosmath.BigDec
}{
	{
		name:  "empty slice",
		input: []cosmosmath.BigDec{},
		min:   cosmosmath.BigDec{},
		max:   cosmosmath.BigDec{},
	},
	{
		name:  "single element",
		input: []cosmosmath.BigDec{cosmosmath.NewBigDec(5)},
		min:   cosmosmath.NewBigDec(5),
		max:   cosmosmath.NewBigDec(5),
	},
	{
		name: "multiple elements, positive",
		input: []cosmosmath.BigDec{
			cosmosmath.NewBigDec(5),
			cosmosmath.NewBigDec(3),
			cosmosmath.NewBigDec(7),
		},
		min: cosmosmath.NewBigDec(3),
		max: cosmosmath.NewBigDec(7),
	},
	{
		name: "multiple elements, mixed",
		input: []cosmosmath.BigDec{
			cosmosmath.NewBigDec(-5),
			cosmosmath.NewBigDec(3),
			cosmosmath.NewBigDec(0),
		},
		min: cosmosmath.NewBigDec(-5),
		max: cosmosmath.NewBigDec(3),
	},
	{
		name: "multiple elements, with decimals",
		input: []cosmosmath.BigDec{
			cosmosmath.MustNewBigDecFromStr("1.5"),
			cosmosmath.MustNewBigDecFromStr("1.05"),
			cosmosmath.MustNewBigDecFromStr("1.55"),
		},
		min: cosmosmath.MustNewBigDecFromStr("1.05"),
		max: cosmosmath.MustNewBigDecFromStr("1.55"),
	},
	{
		name: "multiple elements, all equal",
		input: []cosmosmath.BigDec{
			cosmosmath.NewBigDec(5),
			cosmosmath.NewBigDec(5),
			cosmosmath.NewBigDec(5),
		},
		min: cosmosmath.NewBigDec(5),
		max: cosmosmath.NewBigDec(5),
	},
}

func TestMinBigDec(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := MinBigDec(tc.input...)
			require.True(t, result.Equal(tc.min), "Expected %s, got %s", tc.min, result)
		})
	}
}

func TestMaxBigDec(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := MaxBigDec(tc.input...)
			require.True(t, result.Equal(tc.max), "Expected %s, got %s", tc.max, result)
		})
	}
}
