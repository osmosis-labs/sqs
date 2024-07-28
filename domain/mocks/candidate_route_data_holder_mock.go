package mocks

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type CandidateRouteSearchDataHolderMock struct {
	CandidateRouteSearchData map[string]domain.CandidateRouteDenomData
}

var _ mvc.CandidateRouteSearchDataHolder = &CandidateRouteSearchDataHolderMock{}

// GetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) GetCandidateRouteSearchData() map[string]domain.CandidateRouteDenomData {
	return c.CandidateRouteSearchData
}

// SetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) SetCandidateRouteSearchData(candidateRouteSearchData map[string]domain.CandidateRouteDenomData) {
	c.CandidateRouteSearchData = candidateRouteSearchData
}

// GetDenomData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) GetDenomData(denom string) (domain.CandidateRouteDenomData, error) {
	denomData, ok := c.CandidateRouteSearchData[denom]
	if !ok {
		return domain.CandidateRouteDenomData{}, nil
	}
	return denomData, nil
}
