package passthroughdomain

import (
	"net/http"
	"time"

	sqspassthroughdomain "github.com/osmosis-labs/osmosis/v28/ingest/types/passthroughdomain"
	"github.com/osmosis-labs/sqs/sqsutil/sqshttp"
)

type NumiaHTTPClient interface {
	// GetPoolAPRsRange returns the APR data of the pools as ranges
	GetPoolAPRsRange() ([]sqspassthroughdomain.PoolAPR, error)
}

type NumiaHTTPClientImpl struct {
	client *http.Client
	url    string
}

var _ NumiaHTTPClient = &NumiaHTTPClientImpl{}

const (
	poolAPRRangeEndpoint = "/pools_apr_range"

	// numiaHTTPClientTimeout bounds each request to Numia. Without it a hung connection
	// blocks the APR fetcher indefinitely, and retries never get a chance to run.
	numiaHTTPClientTimeout = 30 * time.Second
)

func NewNumiaHTTPClient(url string) *NumiaHTTPClientImpl {
	return &NumiaHTTPClientImpl{
		client: &http.Client{Timeout: numiaHTTPClientTimeout},
		url:    url,
	}
}

// GetPoolAPRsRange implements NumiaHTTPClient.
func (n *NumiaHTTPClientImpl) GetPoolAPRsRange() ([]sqspassthroughdomain.PoolAPR, error) {
	poolAPR, err := sqshttp.Get[[]sqspassthroughdomain.PoolAPR](n.client, n.url, poolAPRRangeEndpoint)
	if err != nil {
		return nil, err
	}
	return *poolAPR, nil
}
