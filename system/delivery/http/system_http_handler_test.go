package http_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/system/delivery/http"
	"github.com/stretchr/testify/require"
)

func TestExtractVersion(t *testing.T) {

	// Test cases
	testCases := []struct {
		name            string
		ldFlagsValue    string
		expectedVersion string
	}{
		{
			name:         "version is specified first in the ldFlagsValue",
			ldFlagsValue: "-X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8     -w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static'",

			expectedVersion: "0.1.2-4-g79c82c8",
		},
		{
			name:         "version is specified in the end of ldFlagsValue",
			ldFlagsValue: "-w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static' -X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8",

			expectedVersion: "0.1.2-4-g79c82c8",
		},
		{
			name:         "version is specified in the middle of ldFlagsValue",
			ldFlagsValue: "-extldflags '-Wl,-z,muldefs -static' -X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8 -w -s -linkmode=external",

			expectedVersion: "0.1.2-4-g79c82c8",
		},
		{
			name:         "ldFlagsValue only version",
			ldFlagsValue: "-X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8",

			expectedVersion: "0.1.2-4-g79c82c8",
		},
	}

	// Run tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := http.ExtractVersion(tc.ldFlagsValue)
			require.NoError(t, err)

			require.Equal(t, tc.expectedVersion, result)
		})
	}
}
