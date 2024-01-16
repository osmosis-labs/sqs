package http_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/system/delivery/http"
	"github.com/stretchr/testify/require"
)

func TestExtractVersion(t *testing.T) {
	const (
		ldFlagsValue = "-X github.com/osmosis-labs/sqs/version=0.1.2-4-g79c82c8     -w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static'"

		expectedVersion = "0.1.2-4-g79c82c8"
	)

	result, err := http.ExtractVersion(ldFlagsValue)
	require.NoError(t, err)

	require.Equal(t, expectedVersion, result)
}
