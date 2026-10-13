package http_test

import (
	"context"
	"net/http"
	"testing"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"

	"github.com/stretchr/testify/require"
)

// TestWithShutdownSignalRepeatedShutdown checks that calling Shutdown more than once
// does not panic by closing the shutdown channel twice.
func TestWithShutdownSignalRepeatedShutdown(t *testing.T) {
	server := &http.Server{}
	deliveryhttp.WithShutdownSignal(server)

	require.NotPanics(t, func() {
		require.NoError(t, server.Shutdown(context.Background()))
		require.NoError(t, server.Shutdown(context.Background()))
	})
}
