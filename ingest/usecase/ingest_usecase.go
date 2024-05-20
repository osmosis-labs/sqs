package usecase

import (
	"context"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/sqsdomain/json"
	"github.com/osmosis-labs/sqs/sqsdomain/proto/types"
)

type ingestUseCase struct {
	codec codec.Codec

	poolsUseCase     mvc.PoolsUsecase
	routerUsecase    mvc.RouterUsecase
	tokensUsecase    mvc.TokensUsecase
	chainInfoUseCase mvc.ChainInfoUsecase

	denomLiquidityMap domain.DenomLiquidityMap

	// Worker that computes prices for all tokens with the default quote.
	defaultQuotePriceUpdateWorker domain.PricingWorker

	logger log.Logger
}

type poolResult struct {
	pool sqsdomain.PoolI
	err  error
}

var (
	_ mvc.IngestUsecase = &ingestUseCase{}
)

// NewIngestUsecase will create a new pools use case object
func NewIngestUsecase(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, chainInfoUseCase mvc.ChainInfoUsecase, codec codec.Codec, quotePriceUpdateWorker domain.PricingWorker, logger log.Logger) (mvc.IngestUsecase, error) {
	return &ingestUseCase{
		codec: codec,

		chainInfoUseCase: chainInfoUseCase,
		routerUsecase:    routerUseCase,
		tokensUsecase:    tokensUseCase,
		poolsUseCase:     poolsUseCase,

		denomLiquidityMap: make(domain.DenomLiquidityMap),

		logger: logger,

		defaultQuotePriceUpdateWorker: quotePriceUpdateWorker,
	}, nil
}

func (p *ingestUseCase) ProcessBlockData(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap, poolData []*types.PoolData) (err error) {
	p.logger.Info("starting block processing", zap.Uint64("height", height))

	startProcessingTime := time.Now()

	p.routerUsecase.SetTakerFees(takerFeesMap)

	// Parse the pools
	pools, uniqueBlockPoolMetadata, err := p.parsePoolData(ctx, poolData)
	if err != nil {
		return err
	}

	// Store the pools
	if err := p.poolsUseCase.StorePools(pools); err != nil {
		return err
	}

	// Get all pools (already updated with the newly ingested pools)
	allPools, err := p.poolsUseCase.GetAllPools()
	if err != nil {
		return err
	}

	// Sort and store pools.
	p.logger.Info("sorting pools", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(startProcessingTime)))

	p.sortAndStorePools(allPools)

	// Update pool denom metadata
	p.logger.Info("updating pool denom metadata", zap.Uint64("height", height), zap.Int("denom_count", len(uniqueBlockPoolMetadata.DenomLiquidityMap)), zap.Duration("duration_since_start", time.Since(startProcessingTime)))

	// Note: we must queue the update before we start updating prices as pool liquidity
	// worker listens for the pricing updates at the same height.
	p.defaultQuotePriceUpdateWorker.UpdatePricesAsync(height, uniqueBlockPoolMetadata)

	// Store the latest ingested height.
	p.chainInfoUseCase.StoreLatestHeight(height)

	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(startProcessingTime)))

	// Observe the processing duration with height
	domain.SQSIngestHandlerProcessBlockDurationGauge.Add(float64(time.Since(startProcessingTime).Milliseconds()))

	return nil
}

// sortAndStorePools sorts the pools and stores them in the router.
// TODO: instead of resorting all pools every block, we should put the updated pools in the correct position
func (p *ingestUseCase) sortAndStorePools(pools []sqsdomain.PoolI) {
	cosmWasmPoolConfig := p.poolsUseCase.GetCosmWasmPoolConfig()
	routerConfig := p.routerUsecase.GetConfig()

	sortedPools := routerusecase.ValidateAndSortPools(pools, cosmWasmPoolConfig, routerConfig.PreferredPoolIDs, p.logger)

	// Sort the pools and store them in the router.
	p.routerUsecase.SetSortedPools(sortedPools)
}

// parsePoolData parses the pool data and returns the pool objects.
func (p *ingestUseCase) parsePoolData(ctx context.Context, poolData []*types.PoolData) ([]sqsdomain.PoolI, domain.BlockPoolMetadata, error) {
	poolResultChan := make(chan poolResult, len(poolData))

	// Parse the pools concurrently
	for _, pool := range poolData {
		go func(pool *types.PoolData) {
			poolResultData, err := p.parsePool(pool)

			poolResultChan <- poolResult{
				pool: poolResultData,
				err:  err,
			}
		}(pool)
	}

	parsedPools := make([]sqsdomain.PoolI, 0, len(poolData))

	uniqueData := domain.BlockPoolMetadata{
		PoolIDs: make(map[uint64]struct{}, len(poolData)),
	}

	currentBlockLiquidityMap := domain.DenomLiquidityMap{}

	// Collect the parsed pools
	for i := 0; i < len(poolData); i++ {
		select {
		case poolResult := <-poolResultChan:
			if poolResult.err != nil {
				// Increment parse pool error counter
				domain.SQSIngestHandlerPoolParseErrorCounter.WithLabelValues(poolResult.err.Error()).Inc()

				continue
			}

			currentPoolBalances := poolResult.pool.GetSQSPoolModel().Balances

			// Update unique denoms.
			for _, coin := range currentPoolBalances {
				denomData, ok := currentBlockLiquidityMap[coin.Denom]

				updatedLiquidity := osmomath.ZeroInt()
				pools := map[uint64]osmomath.Int{}
				if ok {
					updatedLiquidity = denomData.TotalLiquidity
					for k, v := range denomData.Pools {
						pools[k] = v
					}
				}

				// Update the current pool liquidity for the denom.
				pools[poolResult.pool.GetId()] = coin.Amount
				// Update the total liquidity for the denom.
				updatedLiquidity = updatedLiquidity.Add(coin.Amount)

				currentBlockLiquidityMap[coin.Denom] = domain.DenomLiquidityData{
					TotalLiquidity: updatedLiquidity,
					Pools:          pools,
				}
			}

			// Update unique pools.
			uniqueData.PoolIDs[poolResult.pool.GetId()] = struct{}{}

			parsedPools = append(parsedPools, poolResult.pool)
		case <-ctx.Done():
			return nil, domain.BlockPoolMetadata{}, ctx.Err()
		}
	}

	// Transfer the updated liquidity data to the global map.
	// Note, the updated liquidity data contains updates only for the pools updated
	// in the current block. We need to merge this data with the existing data.
	for denom, currentDenomLiquidityData := range currentBlockLiquidityMap {
		fullLiquidityDataForDenom, ok := p.denomLiquidityMap[denom]
		if !ok {
			p.denomLiquidityMap[denom] = currentBlockLiquidityMap[denom]
			continue
		}

		for poolID, liquidity := range currentDenomLiquidityData.Pools {
			// Current pool data

			currentPoolLiquidity, ok := fullLiquidityDataForDenom.Pools[poolID]
			if ok {
				// Subtract the existing liquidity from the total liquidity.
				fullLiquidityDataForDenom.TotalLiquidity = fullLiquidityDataForDenom.TotalLiquidity.Sub(currentPoolLiquidity)
			}

			// Add the new liquidity to the total liquidity.
			fullLiquidityDataForDenom.TotalLiquidity = fullLiquidityDataForDenom.TotalLiquidity.Add(liquidity)
			// Overwrite liquidity for the pool or set it if it doesn't exist.
			fullLiquidityDataForDenom.Pools[poolID] = liquidity
		}

		// Update the global map with the updated data.
		p.denomLiquidityMap[denom] = fullLiquidityDataForDenom
	}

	// Update unique denoms.
	uniqueData.DenomLiquidityMap = p.denomLiquidityMap

	return parsedPools, uniqueData, nil
}

// parsePool parses the pool data and returns the pool object
// For concentrated pools, it also processes the tick model
func (p *ingestUseCase) parsePool(pool *types.PoolData) (sqsdomain.PoolI, error) {
	poolWrapper := sqsdomain.PoolWrapper{}

	if err := p.codec.UnmarshalInterfaceJSON(pool.ChainModel, &poolWrapper.ChainModel); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(pool.SqsModel, &poolWrapper.SQSModel); err != nil {
		return nil, err
	}

	if poolWrapper.GetType() == poolmanagertypes.Concentrated {
		poolWrapper.TickModel = &sqsdomain.TickModel{}
		if err := json.Unmarshal(pool.TickModel, poolWrapper.TickModel); err != nil {
			return nil, err
		}
	}

	return &poolWrapper, nil
}
