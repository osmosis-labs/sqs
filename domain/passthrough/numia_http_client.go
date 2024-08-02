package passthroughdomain

import (
	"fmt"
	"io"
	"net/http"

	"github.com/osmosis-labs/sqs/sqsdomain/json"
)

type NumiaHTTPClient interface {
	// GetPoolAPRsRange returns the APR data of the pools as ranges
	GetPoolAPRsRange() ([]PoolAPR, error)
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
func (n *NumiaHTTPClientImpl) GetPoolAPRsRange() ([]PoolAPR, error) {
	resp, err := n.client.Get(n.url + poolAPRRangeEndpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unmarshal the response body
	var poolAPRs []PoolAPR
	if err := json.Unmarshal(body, &poolAPRs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal pool APRs: %w", err)
	}

	return poolAPRs, nil
}
