package mocks

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type CandidateRouteSearchDataHolderMock struct {
	CandidateRouteSearchData map[string]*domain.CandidateRouteDenomData
	Error                    error
}

var _ mvc.CandidateRouteSearchDataHolder = &CandidateRouteSearchDataHolderMock{}

// GetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) GetCandidateRouteSearchData() (map[string]*domain.CandidateRouteDenomData, error) {
	return c.CandidateRouteSearchData, c.Error
}

// SetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) SetCandidateRouteSearchData(candidateRouteSearchData map[string]*domain.CandidateRouteDenomData) {
	c.CandidateRouteSearchData = candidateRouteSearchData
}

// GetDenomData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) GetDenomData(denom string) (*domain.CandidateRouteDenomData, error) {
	denomData, ok := c.CandidateRouteSearchData[denom]
	if !ok {
		return &domain.CandidateRouteDenomData{}, nil
	}
	return denomData, nil
}
