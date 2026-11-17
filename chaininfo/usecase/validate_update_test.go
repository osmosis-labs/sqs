package usecase

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Validates the update height check, including heights below the threshold, where an
// unsigned subtraction of the threshold from the ingested height would wrap around.
func TestValidateUpdate(t *testing.T) {
	tests := []struct {
		name string

		currentUpdateHeight  uint64
		latestIngestedHeight uint64

		expectError bool
	}{
		{
			name:                 "no update received yet",
			currentUpdateHeight:  initialUpdateHeight,
			latestIngestedHeight: 100,
			expectError:          true,
		},
		{
			name:                 "update at the ingested height",
			currentUpdateHeight:  1000,
			latestIngestedHeight: 1000,
		},
		{
			name:                 "update exactly at the threshold behind",
			currentUpdateHeight:  1000,
			latestIngestedHeight: 1000 + updateHeightThreshold,
		},
		{
			name:                 "update beyond the threshold behind",
			currentUpdateHeight:  1000,
			latestIngestedHeight: 1000 + updateHeightThreshold + 1,
			expectError:          true,
		},
		{
			name:                 "ingested height below the threshold",
			currentUpdateHeight:  3,
			latestIngestedHeight: 5,
		},
		{
			name:                 "ingested height not observed yet",
			currentUpdateHeight:  3,
			latestIngestedHeight: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateUpdate(tc.currentUpdateHeight, tc.latestIngestedHeight, pricingUpdateName)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
