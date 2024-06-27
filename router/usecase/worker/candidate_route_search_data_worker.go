package worker

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type candidateRouteSearchDataWorker struct {
	listeners                []domain.CandidateRouteSearchDataUpdateListener
	poolsHandler             mvc.PoolHandler
	candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder
	logger                   log.Logger
}

var (
	_ domain.CandidateRouteSearchDataWorker = &candidateRouteSearchDataWorker{}
	_ domain.PoolLiquidityComputeListener   = &candidateRouteSearchDataWorker{}
)

func NewCandidateRouteSearchDataWorker(poolHandler mvc.PoolHandler, candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder, logger log.Logger) *candidateRouteSearchDataWorker {
	return &candidateRouteSearchDataWorker{
		listeners:                []domain.CandidateRouteSearchDataUpdateListener{},
		poolsHandler:             poolHandler,
		candidateRouteDataHolder: candidateRouteDataHolder,
		logger:                   logger,
	}
}

// OnPoolLiquidityCompute implements domain.PoolLiquidityComputeListener.
func (c *candidateRouteSearchDataWorker) OnPoolLiquidityCompute(ctx context.Context, height uint64, blockPoolMetaData domain.BlockPoolMetadata) error {

	// Compute search data and propagate error up the chain to fail the health check.
	if err := c.ComputeSearchData(ctx, height, blockPoolMetaData); err != nil {
		return err
	}

	// Notify listeners
	for _, listener := range c.listeners {
		_ = listener.OnSearchDataUpdate(ctx, height)
	}

	return nil
}

// ComputeSearchData implements domain.CandidateRouteSearchDataWorker.
func (c *candidateRouteSearchDataWorker) ComputeSearchData(ctx context.Context, height uint64, blockPoolMetaData domain.BlockPoolMetadata) error {

	// TODO: implement
	// https://linear.app/osmosis/issue/DATA-248/[candidaterouteopt]-implement-and-test-core-pre-computation-logic

	candidateRouteData := map[string][]sqsdomain.PoolI{}

	c.candidateRouteDataHolder.SetCandidateRouteSearchData(candidateRouteData)

	return nil
}

// RegisterListener implements domain.CandidateRouteSearchDataWorker.
func (c *candidateRouteSearchDataWorker) RegisterListener(listener domain.CandidateRouteSearchDataUpdateListener) {
	c.listeners = append(c.listeners, listener)
}
