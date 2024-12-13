package mocks

import (
	"github.com/cosmos/cosmos-sdk/types"
	ingesttypes "github.com/osmosis-labs/osmosis/v28/ingest/types"
	"github.com/osmosis-labs/sqs/domain"
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
