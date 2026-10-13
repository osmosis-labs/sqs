package datafetchers

import (
	"errors"
	"testing"

	sqspassthroughdomain "github.com/osmosis-labs/osmosis/v28/ingest/types/passthroughdomain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/stretchr/testify/require"
)

// numiaClientStub returns the queued results of GetPoolAPRsRange in order.
type numiaClientStub struct {
	errs  []error
	calls int
}

func (n *numiaClientStub) GetPoolAPRsRange() ([]sqspassthroughdomain.PoolAPR, error) {
	err := n.errs[n.calls]
	n.calls++
	if err != nil {
		return nil, err
	}
	return []sqspassthroughdomain.PoolAPR{{PoolID: 1}}, nil
}

// TestGetFetchPoolAPRsFromNumiaCb verifies that a transient failure is retried,
// and that once every attempt fails the last error is returned.
func TestGetFetchPoolAPRsFromNumiaCb(t *testing.T) {
	t.Parallel()

	t.Run("succeeds after a failed attempt", func(t *testing.T) {
		t.Parallel()

		client := &numiaClientStub{errs: []error{errors.New("timeout"), nil}}

		poolAPRs, err := GetFetchPoolAPRsFromNumiaCb(client, &log.NoOpLogger{})()
		require.NoError(t, err)
		require.Contains(t, poolAPRs, uint64(1))
		require.Equal(t, 2, client.calls)
	})

	t.Run("returns the last error after every attempt fails", func(t *testing.T) {
		t.Parallel()

		lastErr := errors.New("third failure")
		client := &numiaClientStub{errs: []error{errors.New("first failure"), errors.New("second failure"), lastErr}}

		poolAPRs, err := GetFetchPoolAPRsFromNumiaCb(client, &log.NoOpLogger{})()
		require.ErrorIs(t, err, lastErr)
		require.Nil(t, poolAPRs)
		require.Equal(t, numiaAPRsFetchRetries, client.calls)
	})
}
