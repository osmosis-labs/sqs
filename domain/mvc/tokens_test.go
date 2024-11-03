package mvc_test

import (
	"errors"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	denom    = "uatom"
	ibcDenom = "ibc/" + denom
)

func TestValidateChainDenomQueryParam(t *testing.T) {

	baseDenom, err := sdk.GetBaseDenom()
	require.NoError(t, err)

	defaultError := errors.New("default error")

	// Test cases
	tests := []struct {
		name                    string
		denom                   string
		isHumanDenoms           bool
		doesAllowUnlistedDenoms bool
		mock                    *mocks.TokensUsecaseMock
		expectedDenom           string
		expectedError           string
		baseError               bool // true if we expect base denom error
	}{
		{
			name:                    "Error validating base denom",
			denom:                   "not" + baseDenom,
			isHumanDenoms:           true,
			doesAllowUnlistedDenoms: true,
			mock: &mocks.TokensUsecaseMock{
				GetChainDenomFunc: func(humanDenom string) (string, error) {
					return "", defaultError
				},
			},
			expectedDenom: "",
			expectedError: defaultError.Error(),
		},
		{
			name:                    "Human denom conversion success",
			denom:                   denom,
			isHumanDenoms:           true,
			doesAllowUnlistedDenoms: true,
			mock: &mocks.TokensUsecaseMock{
				GetChainDenomFunc: func(humanDenom string) (string, error) {
					return "ibc/" + humanDenom, nil
				},
			},
			expectedDenom: ibcDenom,
			expectedError: "",
		},
		{
			name:                    "Human denom conversion success",
			denom:                   denom,
			isHumanDenoms:           true,
			doesAllowUnlistedDenoms: true,
			mock: &mocks.TokensUsecaseMock{
				GetChainDenomFunc: func(humanDenom string) (string, error) {
					return ibcDenom, nil
				},
			},
			expectedDenom: ibcDenom,
			expectedError: "",
		},
		{
			name:                    "Human denom conversion error",
			denom:                   "invalid",
			isHumanDenoms:           true,
			doesAllowUnlistedDenoms: true,
			mock: &mocks.TokensUsecaseMock{
				GetChainDenomFunc: func(humanDenom string) (string, error) {
					return "", fmt.Errorf("invalid denom")
				},
			},
			expectedDenom: "",
			expectedError: "invalid denom",
		},
		{
			name:                    "Valid unlisted chain denom",
			denom:                   ibcDenom,
			isHumanDenoms:           false,
			doesAllowUnlistedDenoms: true,
			mock: &mocks.TokensUsecaseMock{
				IsValidChainDenomFunc: func(chainDenom string) bool {
					return true
				},
			},
			expectedDenom: ibcDenom,
			expectedError: "",
		},
		{
			name:                    "Invalid chain denom with unlisted allowed",
			denom:                   "invalid",
			isHumanDenoms:           false,
			doesAllowUnlistedDenoms: true,
			mock: &mocks.TokensUsecaseMock{
				IsValidChainDenomFunc: func(chainDenom string) bool {
					return false
				},
			},
			expectedDenom: "",
			expectedError: "denom is not a valid chain denom (invalid)",
		},
		{
			name:                    "Unlisted chain denom when not allowed",
			denom:                   ibcDenom,
			isHumanDenoms:           false,
			doesAllowUnlistedDenoms: false,
			mock: &mocks.TokensUsecaseMock{
				IsValidListedDenomFunc: func(denom string) bool {
					return false
				},
			},
			expectedDenom: "",
			expectedError: "denom is not a valid listed chain denom (" + ibcDenom + ")",
		},
		{
			name:                    "Valid listed chain denom",
			denom:                   ibcDenom,
			isHumanDenoms:           false,
			doesAllowUnlistedDenoms: false,
			mock: &mocks.TokensUsecaseMock{
				IsValidListedDenomFunc: func(denom string) bool {
					return true
				},
			},
			expectedDenom: ibcDenom,
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// System under test
			result, err := mvc.ValidateChainDenomQueryParam(tt.mock, tt.denom, tt.isHumanDenoms, tt.doesAllowUnlistedDenoms)

			// Assert results
			if tt.baseError {
				assert.Empty(t, result)
				assert.NoError(t, err)
			} else if tt.expectedError != "" {
				assert.Empty(t, result)
				assert.EqualError(t, err, tt.expectedError)
			} else {
				assert.Equal(t, tt.expectedDenom, result)
				assert.NoError(t, err)
			}
		})
	}
}
