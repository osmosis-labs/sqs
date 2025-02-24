package worker

import (
	"context"
	"sync"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"

	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
)

type candidateRouteSearchDataWorker struct {
	listeners                []domain.CandidateRouteSearchDataUpdateListener
	poolsHandler             mvc.CandidateRouteSearchPoolHandler
	candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder
	preferredPoolIDs         []uint64
	cosmWasmPoolConfig       domain.CosmWasmPoolRouterConfig
	logger                   log.Logger
}

var (
	_ domain.CandidateRouteSearchDataWorker = &candidateRouteSearchDataWorker{}
)

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

// [debug-adapter stdout] 2025-02-24T12:05:26.247+0200	ERROR	error validating and filtering routes	{"error": "previous token out denom (ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4) not found in pool (2549), route index (0)", "token_in_denom": "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4", "token_out_denom": "ibc/D79E7D83AB399BFFF93433E54FAA480C191248FC556924A2A8351AE2638B3877"}
func (c *candidateRouteSearchDataWorker) compute(blockPoolMetaData domain.BlockPoolMetadata) error {
	mu := sync.Mutex{}

	candidateRouteData := c.candidateRouteDataHolder.GetCandidateRouteSearchData()

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
			for i, pool := range sortedDenomPools {
				if pool.GetId() == uint64(1423) || pool.GetId() == uint64(1570) {
					c.logger.Info("1570_1423", zap.Uint64("pool_id", pool.GetId()), zap.String("denom", denom), zap.Int("index", i))
				}
			}

			if denom == "ibc/831F0B1BBB1D08A2B75311892876D71565478C532967545476DF4C2D7492E48C" {
				sortedDenomPools[8], sortedDenomPools[9] = sortedDenomPools[9], sortedDenomPools[8]
			}
			if denom == "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4" {
				p397 := sortedDenomPools[397]
				p381 := sortedDenomPools[381]

				sortedDenomPools[381] = p397
				sortedDenomPools[397] = p381
			}

			canonicalOrderbookPoolMapByPairToken := make(map[string]ingesttypes.PoolI, len(orderbookPools))
			for _, pool := range orderbookPools {
				if c.poolsHandler.IsCanonicalOrderbookPool(pool.GetId()) {
					poolDenoms := pool.GetPoolDenoms()

					for _, poolDenom := range poolDenoms {
						canonicalOrderbookPoolMapByPairToken[poolDenom] = pool
					}
				}
			}

			mu.Lock()
			candidateRouteData[denom] = domain.CandidateRouteDenomData{
				SortedPools:         sortedDenomPools,
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
