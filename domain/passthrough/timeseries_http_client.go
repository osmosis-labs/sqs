package passthroughdomain

import (
	"net/http"
	"time"

	sqspassthroughdomain "github.com/osmosis-labs/osmosis/v31/ingest/types/passthroughdomain"
	"github.com/osmosis-labs/sqs/sqsutil/sqshttp"
)

type TimeSeriesHTTPClient interface {
	GetPoolFees() (*sqspassthroughdomain.PoolFees, error)
}

type TimeSeriesHTTPClientImpl struct {
	url    string
	client *http.Client
}

const (
	feesEndpoint = "/fees/v1/pools"

	// timeseriesHTTPClientTimeout bounds each request to the timeseries API. Without it a hung
	// connection blocks the pool fees fetcher indefinitely.
	timeseriesHTTPClientTimeout = 30 * time.Second
)

var _ TimeSeriesHTTPClient = &TimeSeriesHTTPClientImpl{}

func NewTimeSeriesHTTPClient(url string) *TimeSeriesHTTPClientImpl {
	return &TimeSeriesHTTPClientImpl{
		url:    url,
		client: &http.Client{Timeout: timeseriesHTTPClientTimeout},
	}
}

// GetPoolFees implements TimeSeriesHTTPClient.
func (t *TimeSeriesHTTPClientImpl) GetPoolFees() (*sqspassthroughdomain.PoolFees, error) {
	poolFees, err := sqshttp.Get[sqspassthroughdomain.PoolFees](t.client, t.url, feesEndpoint)
	if err != nil {
		return nil, err
	}
	return poolFees, nil
}
