package mocks

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type CandidateRouteSearchDataHolderMock struct {
	CandidateRouteSearchData domain.CandidateRouteSearchData
	Error                    error
}

var _ mvc.CandidateRouteSearchDataHolder = &CandidateRouteSearchDataHolderMock{}

// GetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) GetCandidateRouteSearchData() (domain.CandidateRouteSearchData, error) {
	return c.CandidateRouteSearchData, c.Error
}

// SetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *CandidateRouteSearchDataHolderMock) SetCandidateRouteSearchData(candidateRouteSearchData domain.CandidateRouteSearchData) {
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
