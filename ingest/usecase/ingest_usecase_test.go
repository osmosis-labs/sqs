package usecase_test

import (
	"reflect"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

var (
	UOSMO = routertesting.UOSMO
	USDC  = routertesting.USDC
)

type IngestUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

func TestIngestUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(IngestUseCaseTestSuite))
}

// TestUpdateUniqueDenomData provides table-driven tests for the updateUniqueDenomData function
func TestUpdateUniqueDenomData(t *testing.T) {
	// Test case structure
	type test struct {
		name            string
		uniqueDenomData map[string]domain.PoolDenomMetaData
		balances        sdk.Coins
		expected        map[string]domain.PoolDenomMetaData
	}

	var (
		amountX     = osmomath.NewInt(100)
		amountHalfX = osmomath.NewInt(50)

		sumOfAmounts = amountX.Add(amountHalfX)
	)

	tests := []test{
		{
			name:            "Empty map, new entries added",
			uniqueDenomData: map[string]domain.PoolDenomMetaData{},
			balances:        []sdk.Coin{{Denom: UOSMO, Amount: amountX}, {Denom: USDC, Amount: amountHalfX}},
			expected:        map[string]domain.PoolDenomMetaData{UOSMO: {LocalMCap: amountX}, USDC: {LocalMCap: amountHalfX}},
		},
		{
			name:            "Existing entries, updated correctly",
			uniqueDenomData: map[string]domain.PoolDenomMetaData{UOSMO: {LocalMCap: amountX}},
			balances:        []sdk.Coin{{Denom: UOSMO, Amount: amountHalfX}},
			expected:        map[string]domain.PoolDenomMetaData{UOSMO: {LocalMCap: amountX.Add(amountHalfX)}},
		},
		{
			name:            "Mix of new and update entries",
			uniqueDenomData: map[string]domain.PoolDenomMetaData{UOSMO: {LocalMCap: amountX}},
			balances:        []sdk.Coin{{Denom: UOSMO, Amount: amountHalfX}, {Denom: USDC, Amount: amountHalfX}},
			expected:        map[string]domain.PoolDenomMetaData{UOSMO: {LocalMCap: sumOfAmounts}, USDC: {LocalMCap: amountHalfX}},
		},
		{
			name:            "No balances provided, map unchanged",
			uniqueDenomData: map[string]domain.PoolDenomMetaData{UOSMO: {LocalMCap: amountX}},
			balances:        []sdk.Coin{},
			expected:        map[string]domain.PoolDenomMetaData{UOSMO: {LocalMCap: amountX}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			usecase.UpdateUniqueDenomData(tc.uniqueDenomData, tc.balances)
			if !reflect.DeepEqual(tc.uniqueDenomData, tc.expected) {
				t.Errorf("Unexpected result for '%s': got %v, want %v", tc.name, tc.uniqueDenomData, tc.expected)
			}
		})
	}
}
