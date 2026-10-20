package datafetchers

import (
	"errors"
	"testing"

	sqspassthroughdomain "github.com/osmosis-labs/osmosis/v31/ingest/types/passthroughdomain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/stretchr/testify/require"
)

// errEmptyAPRsResponse makes numiaClientStub return a successful response with no pools.
var errEmptyAPRsResponse = errors.New("empty response")

// numiaClientStub returns the queued results of GetPoolAPRsRange in order.
type numiaClientStub struct {
	errs  []error
	calls int
}

func (n *numiaClientStub) GetPoolAPRsRange() ([]sqspassthroughdomain.PoolAPR, error) {
	err := n.errs[n.calls]
	n.calls++
	if errors.Is(err, errEmptyAPRsResponse) {
		return []sqspassthroughdomain.PoolAPR{}, nil
	}
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

	t.Run("retries a response with no pools", func(t *testing.T) {
		t.Parallel()

		client := &numiaClientStub{errs: []error{errEmptyAPRsResponse, nil}}

		poolAPRs, err := GetFetchPoolAPRsFromNumiaCb(client, &log.NoOpLogger{})()
		require.NoError(t, err)
		require.Contains(t, poolAPRs, uint64(1))
		require.Equal(t, 2, client.calls)
	})

	t.Run("fails when every response has no pools", func(t *testing.T) {
		t.Parallel()

		client := &numiaClientStub{errs: []error{errEmptyAPRsResponse, errEmptyAPRsResponse, errEmptyAPRsResponse}}

		poolAPRs, err := GetFetchPoolAPRsFromNumiaCb(client, &log.NoOpLogger{})()
		require.Error(t, err)
		require.Nil(t, poolAPRs)
		require.Equal(t, numiaAPRsFetchRetries, client.calls)
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

// timeseriesClientStub returns the queued pool fees responses in order.
type timeseriesClientStub struct {
	responses []*sqspassthroughdomain.PoolFees
	calls     int
}

func (t *timeseriesClientStub) GetPoolFees() (*sqspassthroughdomain.PoolFees, error) {
	response := t.responses[t.calls]
	t.calls++
	if response == nil {
		return nil, errors.New("upstream 500")
	}
	return response, nil
}

// TestGetFetchPoolPoolFeesFromTimeseries verifies that a failed or empty response is retried,
// so neither can replace cached fee data, and that once every attempt fails the error is returned.
func TestGetFetchPoolPoolFeesFromTimeseries(t *testing.T) {
	t.Parallel()

	t.Run("succeeds after a failed and an empty attempt", func(t *testing.T) {
		t.Parallel()

		client := &timeseriesClientStub{responses: []*sqspassthroughdomain.PoolFees{
			nil,
			{},
			{Data: []sqspassthroughdomain.PoolFee{{PoolID: "1", Volume24h: 10}}},
		}}

		poolFees, err := GetFetchPoolPoolFeesFromTimeseries(client, &log.NoOpLogger{})()
		require.NoError(t, err)
		require.Equal(t, 10.0, poolFees[1].Volume24h)
		require.Equal(t, 3, client.calls)
	})

	t.Run("returns an error after every attempt fails", func(t *testing.T) {
		t.Parallel()

		client := &timeseriesClientStub{responses: []*sqspassthroughdomain.PoolFees{nil, nil, {}}}

		poolFees, err := GetFetchPoolPoolFeesFromTimeseries(client, &log.NoOpLogger{})()
		require.Error(t, err)
		require.Nil(t, poolFees)
		require.Equal(t, poolFeesFetchRetries, client.calls)
	})
}
