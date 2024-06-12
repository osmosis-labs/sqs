package usecase

import (
	"context"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"go.uber.org/zap"

	sdk "github.com/cosmos/cosmos-sdk/types"

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
	chainInfoUseCase mvc.ChainInfoUsecase

	denomLiquidityMap domain.DenomPoolLiquidityMap

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
func NewIngestUsecase(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, chainInfoUseCase mvc.ChainInfoUsecase, codec codec.Codec, quotePriceUpdateWorker domain.PricingWorker, logger log.Logger) (mvc.IngestUsecase, error) {
	return &ingestUseCase{
		codec: codec,

		chainInfoUseCase: chainInfoUseCase,
		routerUsecase:    routerUseCase,
		poolsUseCase:     poolsUseCase,

		denomLiquidityMap: make(domain.DenomPoolLiquidityMap),

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
		PoolIDs:       make(map[uint64]struct{}, len(poolData)),
		UpdatedDenoms: make(map[string]struct{}),
	}

	currentBlockLiquidityMap := domain.DenomPoolLiquidityMap{}

	// Collect the parsed pools
	for i := 0; i < len(poolData); i++ {
		select {
		case poolResult := <-poolResultChan:
			if poolResult.err != nil {
				// Increment parse pool error counter
				domain.SQSIngestHandlerPoolParseErrorCounter.WithLabelValues(poolResult.err.Error()).Inc()

				continue
			}

			// Get balances and pool ID.
			currentPoolBalances := poolResult.pool.GetSQSPoolModel().Balances
			poolID := poolResult.pool.GetId()

			// Update block liquidity map.
			currentBlockLiquidityMap = updateCurrentBlockLiquidityMapFromBalances(currentBlockLiquidityMap, currentPoolBalances, poolID)

			// Separately update unique denoms.
			for _, balance := range currentPoolBalances {
				uniqueData.UpdatedDenoms[balance.Denom] = struct{}{}
			}

			// Update unique pools.
			uniqueData.PoolIDs[poolID] = struct{}{}

			parsedPools = append(parsedPools, poolResult.pool)
		case <-ctx.Done():
			return nil, domain.BlockPoolMetadata{}, ctx.Err()
		}
	}

	// Transfer the updated block denom liquidity data to the global map.
	// Note, the updated liquidity data contains updates only for the pools updated
	// in the current block. We need to merge this data with the holistic existing data.
	p.denomLiquidityMap = transferDenomLiquidityMap(p.denomLiquidityMap, currentBlockLiquidityMap)

	// Update unique denoms.
	uniqueData.DenomPoolLiquidityMap = p.denomLiquidityMap

	return parsedPools, uniqueData, nil
}

// updateCurrentBlockLiquidityMapFromBalances updates the current block liquidity map with the balance from the pool of the supplied ID.
// For each denom, if there is pre-existent denom data, it is updated, if there is no denom dat, it is initialized to the given balances.
// CONTRACT: if thehere is a liqudiity entry for a denom, it must have been previously initialized by calling this function.
// Returns the updated map.
func updateCurrentBlockLiquidityMapFromBalances(currentBlockLiquidityMap domain.DenomPoolLiquidityMap, currentPoolBalances sdk.Coins, poolID uint64) domain.DenomPoolLiquidityMap {
	// For evey coin in balance
	for _, coin := range currentPoolBalances {
		// Get denom data for this denom
		denomData, ok := currentBlockLiquidityMap[coin.Denom]
		if !ok {
			// Initialize if does not exist
			denomData = domain.DenomPoolLiquidityData{
				TotalLiquidity: osmomath.ZeroInt(),
				Pools:          map[uint64]osmomath.Int{},
			}
		}

		// Set the denom liquidity contribution from the given pool
		denomData.Pools[poolID] = coin.Amount

		// Update total liquidity
		denomData.TotalLiquidity = denomData.TotalLiquidity.Add(coin.Amount)

		// Update the block liquidity map
		currentBlockLiquidityMap[coin.Denom] = denomData
	}

	// Return for idiomacy despite param mutation.
	return currentBlockLiquidityMap
}

// transferDenomLiquidityMap transfer the updated block denom liquidity data from transferFrom to
// transferTo map.
//
// Note, the updated liquidity data contains updates only for the pools updated
// in the current block. We need to merge this data with the holistic existing data.
//
// Returns the updated map.
//
// Transfer process:
// If there is an entry in transferFrom map that does not exist in transferTo, it is copied to the transferTo map.
// If there is an entry for the same denom in both maps, it is merged from one map to the other.
//
// Merge process:
// For all pools in the transfer from map, if there is an entry for that pool in the transferTo map, we subtract
// that pools liquidity contribution from total in the transferTo map.
//
// We then simply add the transferFrom liquidity map to the total to reflect the new total.
// the updated denom liquidity data is then set for that denom.
func transferDenomLiquidityMap(transferTo, transferFrom domain.DenomPoolLiquidityMap) domain.DenomPoolLiquidityMap {
	for denom, transferFromDenomLiquidityData := range transferFrom {
		transferToLiquidityDataForDenom, ok := transferTo[denom]
		if !ok {
			transferTo[denom] = transferFromDenomLiquidityData
			continue
		}

		// Transfer pools
		for transferFromPoolID, transferFromLiquidity := range transferFromDenomLiquidityData.Pools {
			// Current pool data
			transferToPoolLiquidity, ok := transferToLiquidityDataForDenom.Pools[transferFromPoolID]
			if ok {
				// Subtract the existing liquidity from the total liquidity.
				transferToLiquidityDataForDenom.TotalLiquidity = transferToLiquidityDataForDenom.TotalLiquidity.Sub(transferToPoolLiquidity)
			}

			// Add the new liquidity to the total liquidity.
			transferToLiquidityDataForDenom.TotalLiquidity = transferToLiquidityDataForDenom.TotalLiquidity.Add(transferFromLiquidity)
			// Overwrite liquidity for the pool or set it if it doesn't exist.
			transferToLiquidityDataForDenom.Pools[transferFromPoolID] = transferFromLiquidity
		}

		// Update the global map with the updated data.
		transferTo[denom] = transferToLiquidityDataForDenom
	}

	return transferTo
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
