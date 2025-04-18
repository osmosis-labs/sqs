package mocks

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
)

type CandidateRouteFinderMock struct {
	Routes ingesttypes.CandidateRoutes
	Error  error
}

var _ domain.CandidateRouteSearcher = CandidateRouteFinderMock{}

// FindCandidateRoutesOutGivenIn implements domain.CandidateRouteSearcher.
func (c CandidateRouteFinderMock) FindCandidateRoutesOutGivenIn(ctx context.Context, tokenIn types.Coin, tokenOutDenom string, options domain.CandidateRouteSearchOptions) (ingesttypes.CandidateRoutes, error) {
	return c.Routes, c.Error
}

// FindCandidateRoutesInGivenOut implements domain.CandidateRouteSearcher.
func (c CandidateRouteFinderMock) FindCandidateRoutesInGivenOut(ctx context.Context, tokenOut types.Coin, tokenInDenom string, options domain.CandidateRouteSearchOptions) (ingesttypes.CandidateRoutes, error) {
	return c.Routes, c.Error
}
