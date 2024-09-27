package passthroughdomain

import (
	"net/http"

	sqspassthroughdomain "github.com/osmosis-labs/sqs/sqsdomain/passthroughdomain"
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
)

func NewNumiaHTTPClient(url string) *NumiaHTTPClientImpl {
	return &NumiaHTTPClientImpl{
		client: &http.Client{},
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
