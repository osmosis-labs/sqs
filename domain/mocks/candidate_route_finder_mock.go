package mocks

import (
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type CandidateRouteFinderMock struct {
	Routes sqsdomain.CandidateRoutes
	Error  error
}

var _ domain.CandidateRouteSearcher = CandidateRouteFinderMock{}

// FindCandidateRoutes implements domain.CandidateRouteSearcher.
func (c CandidateRouteFinderMock) FindCandidateRoutes(tokenIn types.Coin, tokenOutDenom string, options domain.CandidateRouteSearchOptions) (sqsdomain.CandidateRoutes, error) {
	return c.Routes, c.Error
}
