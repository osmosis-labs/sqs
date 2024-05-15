package usecase

import (
	"context"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"go.uber.org/zap"

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

	// Worker that computes prices for all tokens with the default quote.
	defaultQuotePriceUpdateWorker domain.PricingWorker

	poolLiquidityPricingWorker domain.PoolLiquidityPricingWorker

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

	// TODO: can this calculation be moved into the pool liquidity worker
	poolDenomLiquidity := p.sortAndStorePools(allPools)

	previousBlockDenomMetadata := p.tokensUsecase.GetFullPoolDenomMetadata()
	for denom, updatedLiquidityMetadata := range poolDenomLiquidity {
		poolDenomLiquidity[denom] = domain.PoolDenomMetaData{
			// Note, we recompute the total liquidity across all pools
			TotalLiquidity: updatedLiquidityMetadata.TotalLiquidity,
			// For tokens that were not updated within this block, we keep the previous value
			TotalLiquidityUSDC: previousBlockDenomMetadata[denom].TotalLiquidityUSDC,
		}
	}

	// Update block pool denom metadata
	uniqueBlockPoolMetadata.DenomLiquidityMap = poolDenomLiquidity

	// Update pool denom metadata
	p.logger.Info("updating pool denom metadata", zap.Uint64("height", height), zap.Int("denom_count", len(uniqueBlockPoolMetadata.DenomLiquidityMap)), zap.Duration("duration_since_start", time.Since(startProcessingTime)))
	p.tokensUsecase.UpdatePoolDenomMetadata(uniqueBlockPoolMetadata.DenomLiquidityMap)

	// Note: we must queue the update before we start updating prices as pool liquidity
	// worker listens for the pricing updates at the same height.
	p.defaultQuotePriceUpdateWorker.UpdatePricesAsync(height, uniqueBlockPoolMetadata)

	// Store the latest ingested height.
	p.chainInfoUseCase.StoreLatestHeight(height)

	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(startProcessingTime)))

	// Observe the processing duration with height
	domain.SQSIngestHandlerProcessBlockDurationHistogram.WithLabelValues(strconv.FormatUint(height, 10)).Observe(float64(time.Since(startProcessingTime).Nanoseconds()))

	return nil
}

// sortAndStorePools sorts the pools and stores them in the router.
// TODO: instead of resorting all pools every block, we should put the updated pools in the correct position
func (p *ingestUseCase) sortAndStorePools(pools []sqsdomain.PoolI) map[string]domain.PoolDenomMetaData {
	cosmWasmPoolConfig := p.poolsUseCase.GetCosmWasmPoolConfig()
	routerConfig := p.routerUsecase.GetConfig()

	sortedPools, poolDenomLiquidity := routerusecase.ValidateAndSortPools(pools, cosmWasmPoolConfig, routerConfig.PreferredPoolIDs, p.logger)

	// Sort the pools and store them in the router.
	p.routerUsecase.SetSortedPools(sortedPools)

	return poolDenomLiquidity
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
		UpdatedDenoms: make(map[string]struct{}, len(poolData)),
		PoolIDs:       make(map[uint64]struct{}, len(poolData)),
	}

	// Collect the parsed pools
	for i := 0; i < len(poolData); i++ {
		select {
		case poolResult := <-poolResultChan:
			if poolResult.err != nil {
				// Increment parse pool error counter
				domain.SQSIngestHandlerPoolParseErrorCounter.WithLabelValues(poolResult.err.Error()).Inc()

				continue
			}

			// Update unique denoms.
			for _, coin := range poolResult.pool.GetSQSPoolModel().Balances {
				uniqueData.UpdatedDenoms[coin.Denom] = struct{}{}
			}

			// Update unique pools.
			uniqueData.PoolIDs[poolResult.pool.GetId()] = struct{}{}

			parsedPools = append(parsedPools, poolResult.pool)
		case <-ctx.Done():
			return nil, domain.BlockPoolMetadata{}, ctx.Err()
		}
	}

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
