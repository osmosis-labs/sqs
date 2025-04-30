package worker

import (
	"context"
	"sync"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"

	"go.uber.org/zap"
)

type candidateRouteSearchDataWorker struct {
	listeners                []domain.CandidateRouteSearchDataUpdateListener
	poolsHandler             mvc.CandidateRouteSearchPoolHandler
	candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder
	preferredPoolIDs         []uint64
	cosmWasmPoolConfig       domain.CosmWasmPoolRouterConfig
	logger                   log.Logger
}

var _ domain.CandidateRouteSearchDataWorker = &candidateRouteSearchDataWorker{}

func NewCandidateRouteSearchDataWorker(poolHandler mvc.CandidateRouteSearchPoolHandler, candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder, preferredPoolIDs []uint64, cosmWasmPoolConfig domain.CosmWasmPoolRouterConfig, logger log.Logger) *candidateRouteSearchDataWorker {
	return &candidateRouteSearchDataWorker{
		listeners:                []domain.CandidateRouteSearchDataUpdateListener{},
		poolsHandler:             poolHandler,
		candidateRouteDataHolder: candidateRouteDataHolder,
		preferredPoolIDs:         preferredPoolIDs,
		cosmWasmPoolConfig:       cosmWasmPoolConfig,
		logger:                   logger,
	}
}

// ComputeSearchDataSync implements domain.CandidateRouteSearchDataWorker.
func (c *candidateRouteSearchDataWorker) ComputeSearchDataAsync(ctx context.Context, height uint64, blockPoolMetaData domain.BlockPoolMetadata) error {
	go func() {
		if err := c.ComputeSearchDataSync(ctx, height, blockPoolMetaData); err != nil {
			c.logger.Error("failed to compute search data", zap.Error(err))
		}
	}()

	return nil
}

// ComputeSearchDataSync implements domain.CandidateRouteSearchDataWorker.
func (c *candidateRouteSearchDataWorker) ComputeSearchDataSync(ctx context.Context, height uint64, blockPoolMetaData domain.BlockPoolMetadata) error {
	// TODO: measure processing time

	if err := c.compute(blockPoolMetaData); err != nil {
		return err
	}

	// Notify listeners
	for _, listener := range c.listeners {
		_ = listener.OnSearchDataUpdate(ctx, height)
	}

	return nil
}

func (c *candidateRouteSearchDataWorker) compute(blockPoolMetaData domain.BlockPoolMetadata) error {
	mu := sync.Mutex{}

	candidateRouteData := make(map[string]*domain.CandidateRouteDenomData, len(blockPoolMetaData.UpdatedDenoms))

	wg := sync.WaitGroup{}

	for denom := range blockPoolMetaData.UpdatedDenoms {
		wg.Add(1)

		go func(denom string) {
			defer wg.Done()

			denomLiquidityData, ok := blockPoolMetaData.DenomPoolLiquidityMap[denom]
			if !ok {
				// TODO: add counter
				c.logger.Error("denom liquidity data not found in candidate route worker", zap.String("denom", denom))
				return
			}

			denomPoolsIDs := domain.KeysFromMap(denomLiquidityData.Pools)

			unsortedDenomPools, _, err := c.poolsHandler.GetPools(
				domain.WithPoolIDFilter(denomPoolsIDs),
			)
			if err != nil {
				// TODO: add counter
				c.logger.Error("failed to get pools in candidate route worker", zap.Error(err))
				return
			}

			// Sort pools
			sortedDenomPools, orderbookPools := routerusecase.ValidateAndSortPools(unsortedDenomPools, c.cosmWasmPoolConfig, c.preferredPoolIDs, c.logger)

			canonicalOrderbookPoolMapByPairToken := make(map[string]domain.CandidatePoolWrapper, len(orderbookPools))
			for i := range orderbookPools {
				id := orderbookPools[i].GetId()
				if c.poolsHandler.IsCanonicalOrderbookPool(id) {
					poolModel := orderbookPools[i].GetSQSPoolModel()
					poolDenoms := poolModel.PoolDenoms
					for _, poolDenom := range poolDenoms {
						canonicalOrderbookPoolMapByPairToken[poolDenom] = domain.NewCandidatePoolWrapper(
							id,
							poolModel,
						)
					}
				}
			}

			mu.Lock()
			sortedPools := make([]domain.CandidatePoolWrapper, len(sortedDenomPools))
			for i := range sortedDenomPools {
				sortedPools[i] = domain.NewCandidatePoolWrapper(
					sortedDenomPools[i].GetId(),
					sortedDenomPools[i].GetSQSPoolModel(),
				)
			}
			candidateRouteData[denom] = &domain.CandidateRouteDenomData{
				SortedPools:         sortedPools,
				CanonicalOrderbooks: canonicalOrderbookPoolMapByPairToken,
			}
			mu.Unlock()
		}(denom)
	}

	wg.Wait()

	c.candidateRouteDataHolder.SetCandidateRouteSearchData(candidateRouteData)

	return nil
}

// RegisterListener implements domain.CandidateRouteSearchDataWorker.
func (c *candidateRouteSearchDataWorker) RegisterListener(listener domain.CandidateRouteSearchDataUpdateListener) {
	c.listeners = append(c.listeners, listener)
}
