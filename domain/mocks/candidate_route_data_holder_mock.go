package mocks

import (
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type CandidateRouteSearchDataHolderMock struct {
	CandidateRouteSearchData map[string][]sqsdomain.PoolI
}

var _ mvc.CandidateRouteSearchDataHolder = &CandidateRouteSearchDataHolderMock{}

// GetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) GetCandidateRouteSearchData() map[string][]sqsdomain.PoolI {
	return c.CandidateRouteSearchData
}

// SetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) SetCandidateRouteSearchData(candidateRouteSearchData map[string][]sqsdomain.PoolI) {
	c.CandidateRouteSearchData = candidateRouteSearchData
}

// GetRankedPoolsByDenom implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) GetRankedPoolsByDenom(denom string) ([]sqsdomain.PoolI, error) {
	pools, ok := c.CandidateRouteSearchData[denom]
	if !ok {
		return []sqsdomain.PoolI{}, nil
	}
	return pools, nil
}
