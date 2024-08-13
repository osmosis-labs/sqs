package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"go.opentelemetry.io/otel"
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

	poolsUseCase         mvc.PoolsUsecase
	routerUsecase        mvc.RouterUsecase
	pricingRouterUsecase mvc.RouterUsecase
	tokensUsecase        mvc.TokensUsecase
	chainInfoUseCase     mvc.ChainInfoUsecase
	orderBookUseCase     mvc.OrderBookUsecase

	denomLiquidityMap domain.DenomPoolLiquidityMap

	// Worker that computes prices for all tokens with the default quote.
	defaultQuotePriceUpdateWorker domain.PricingWorker

	// Worker that computes candidate routes for all tokens.
	candidateRouteSearchWorker domain.CandidateRouteSearchDataWorker

	// endBlockProcessPlugins are the plugins to execute at the end of the block.
	endBlockProcessPlugins []domain.EndBlockProcessPlugin

	// The first height observed after start-up
	// See firstBlockPoolCountThreshold for details.
	firstHeightAfterStartUp atomic.Uint64
	// Wait group to wait for the first block to be processed.
	//
	firstBlockWg sync.WaitGroup

	logger log.Logger
}

type poolResult struct {
	pool sqsdomain.PoolI
	err  error
}

const (
	// The threshold for the number of pools in the first block
	// where we ingest all pools to be ingested.
	// The number is chosen to also be compatible with testnet where
	// the number of pools is 500.
	// In the future, it migth eb beneficial to remove this opinionated mechanism.
	// Today, having a clear way to identify first block
	//  is useful for debugging performance issues when processing it.
	// Note that the first block might arrive later than subsequent due to network or
	// data parsing delays.
	firstBlockPoolCountThreshold = 499

	tracerName = "sqs-ingest-usecase"
)

var (
	tracer = otel.Tracer(tracerName)
)

var (
	_ mvc.IngestUsecase = &ingestUseCase{}
)

// NewIngestUsecase will create a new pools use case object
func NewIngestUsecase(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, pricingRouterUsecase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, chainInfoUseCase mvc.ChainInfoUsecase, codec codec.Codec, quotePriceUpdateWorker domain.PricingWorker, candidateRouteSearchWorker domain.CandidateRouteSearchDataWorker, orderBookUseCase mvc.OrderBookUsecase, logger log.Logger) (mvc.IngestUsecase, error) {
	return &ingestUseCase{
		codec: codec,

		chainInfoUseCase:     chainInfoUseCase,
		routerUsecase:        routerUseCase,
		pricingRouterUsecase: pricingRouterUsecase,
		tokensUsecase:        tokensUseCase,
		poolsUseCase:         poolsUseCase,

		denomLiquidityMap: make(domain.DenomPoolLiquidityMap),

		logger: logger,

		defaultQuotePriceUpdateWorker: quotePriceUpdateWorker,

		orderBookUseCase: orderBookUseCase,

		candidateRouteSearchWorker: candidateRouteSearchWorker,

		firstHeightAfterStartUp: atomic.Uint64{},
	}, nil
}

func (p *ingestUseCase) ProcessBlockData(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap, poolData []*types.PoolData) (err error) {
	ctx, span := tracer.Start(ctx, "ingestUseCase.ProcessBlockData")
	defer span.End()

	if p.firstHeightAfterStartUp.Load() == 0 && len(poolData) > firstBlockPoolCountThreshold {
		p.logger.Info("setting first block height", zap.Uint64("height", height))
		p.firstHeightAfterStartUp.Store(height)
		p.firstBlockWg.Add(1)
	}

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

	// If an error occurs, we should return it and not proceed with the next steps.
	// The pricing relies on the search data. As a result, by returnining an error we trigger a fallback mechanism
	// Note that compute search data is always synchronous because it is needed for all subsequent pre-computations within a block.
	// Its latency is estimated to be negligile. As a result, it is not a concern.
	if err := p.candidateRouteSearchWorker.ComputeSearchDataSync(ctx, height, uniqueBlockPoolMetadata); err != nil {
		p.logger.Error("failed to compute search data", zap.Error(err))
		return err
	}

	if height == p.firstHeightAfterStartUp.Load() {
		// For the first block, we need to update the prices synchronously.
		// and let any subsequent block wait before starting its computation
		// to avoid overloading the system.
		defer p.firstBlockWg.Done()

		// Pre-compute the prices for all tokens
		p.defaultQuotePriceUpdateWorker.UpdatePricesSync(height, uniqueBlockPoolMetadata)

		// Completely reprice the pool liquidity for the first block asyncronously
		// second time.
		// This is necessary because the initial pricing is computed within min liquidity capitalization.
		// That results in a suboptimal price.
		p.defaultQuotePriceUpdateWorker.UpdatePricesAsync(height, uniqueBlockPoolMetadata)

		// Recompute search data given the availability of pool liquidity pricing.
		if err := p.candidateRouteSearchWorker.ComputeSearchDataSync(ctx, height, uniqueBlockPoolMetadata); err != nil {
			p.logger.Error("failed to compute search data", zap.Error(err))
			return err
		}
	} else {
		// Wait for the first block to be processed before
		// updating the prices for the next block.
		p.firstBlockWg.Wait()

		// For any block after the first block, we can update the prices asynchronously.
		p.defaultQuotePriceUpdateWorker.UpdatePricesAsync(height, uniqueBlockPoolMetadata)
	}

	// Store the latest ingested height.
	p.chainInfoUseCase.StoreLatestHeight(height)

	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(startProcessingTime)))

	// We update the assets at the height interval asynchronously to avoid blocking the processing of the next block.
	p.updateAssetsAtHeightIntervalAsync(height)

	// Execute the end block process plugins.
	go p.executeEndBlockProcessPlugins(ctx, height, uniqueBlockPoolMetadata)

	// Observe the processing duration with height
	domain.SQSIngestHandlerProcessBlockDurationGauge.Set(float64(time.Since(startProcessingTime).Milliseconds()))

	return nil
}

// RegisterEndBlockProcessPlugin implements mvc.IngestUsecase.
func (p *ingestUseCase) RegisterEndBlockProcessPlugin(plugin domain.EndBlockProcessPlugin) {
	p.endBlockProcessPlugins = append(p.endBlockProcessPlugins, plugin)
}

// updateAssetsAtHeightIntervalAsync updates the assets at the height interval asynchronously.
// Any error that occurs during the update is recorded in the error counter.
func (p *ingestUseCase) updateAssetsAtHeightIntervalAsync(height uint64) {
	go func() {
		if err := p.tokensUsecase.UpdateAssetsAtHeightIntervalSync(height); err != nil {
			domain.SQSUpdateAssetsAtHeightIntervalErrorCounter.WithLabelValues(err.Error(), fmt.Sprint(height)).Inc()
		}
	}()
}

// sortAndStorePools sorts the pools and stores them in the router.
// TODO: instead of resorting all pools every block, we should put the updated pools in the correct position
func (p *ingestUseCase) sortAndStorePools(pools []sqsdomain.PoolI) {
	cosmWasmPoolConfig := p.poolsUseCase.GetCosmWasmPoolConfig()
	routerConfig := p.routerUsecase.GetConfig()

	sortedPools, _ := routerusecase.ValidateAndSortPools(pools, cosmWasmPoolConfig, routerConfig.PreferredPoolIDs, p.logger)

	// Sort the pools and store them in the router.
	p.routerUsecase.SetSortedPools(sortedPools)
	p.pricingRouterUsecase.SetSortedPools(sortedPools)
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
			sqsModel := poolResult.pool.GetSQSPoolModel()
			currentPoolBalances := sqsModel.Balances
			poolID := poolResult.pool.GetId()

			// Update block liquidity map.
			currentBlockLiquidityMap = updateCurrentBlockLiquidityMapFromBalances(currentBlockLiquidityMap, currentPoolBalances, poolID)

			// Separately update unique denoms.
			for _, balance := range currentPoolBalances {
				if balance.Validate() != nil {
					p.logger.Debug("invalid pool balance", zap.Uint64("pool_id", poolID), zap.String("denom", balance.Denom), zap.String("amount", balance.Amount.String()))
					continue
				}

				uniqueData.UpdatedDenoms[balance.Denom] = struct{}{}
			}

			// Handle the alloyed LP share stemming from the "minting" pools.
			// See updateCurrentBlockLiquidityMapAlloyed for details.
			cosmWasmModel := sqsModel.CosmWasmPoolModel
			if cosmWasmModel != nil && cosmWasmModel.IsAlloyTransmuter() {
				alloyedDenom := cosmWasmModel.Data.AlloyTransmuter.AlloyedDenom
				uniqueData.UpdatedDenoms[alloyedDenom] = struct{}{}

				currentBlockLiquidityMap = updateCurrentBlockLiquidityMapAlloyed(currentBlockLiquidityMap, poolID, alloyedDenom)
			}

			// Process the orderbook pool.
			if cosmWasmModel != nil && cosmWasmModel.IsOrderbook() {
				// Process the orderbook pool.
				if err := p.orderBookUseCase.ProcessPool(ctx, poolResult.pool); err != nil {
					p.logger.Error("failed to process orderbook pool", zap.Error(err), zap.Uint64("pool_id", poolID))
				}
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
		if coin.Validate() != nil {
			// Skip invalid coins.
			// Example: pool 1176 (transmuter v1 pool) has invalid coins.
			// https://celatone.osmosis.zone/osmosis-1/contracts/osmo136f4pv283yywv3t56d5zdkhq43uucw462rt3qfpm2s84vvr7rrasn3kllg
			continue
		}

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

// updateCurrentBlockLiquidityMapAlloyed updates the current block liquidity map with the alloyed LP share.
// Since the LP share is not present in the balances for the "minting" pool, we treat its contribution to
// the liqudity as zero. However, we still create a mapping from the LP share to the pool ID so that
// we can find routes over the minting pools.
// CONTRACT: the pool ID is validated to be an alloyed pool that mints the LP share.
// See `docs/architecture/COSMWASM_POOLS.md` for details.
func updateCurrentBlockLiquidityMapAlloyed(currentBlockLiquidityMap domain.DenomPoolLiquidityMap, poolID uint64, alloyedDenom string) domain.DenomPoolLiquidityMap {
	denomPoolLiquidityData, ok := currentBlockLiquidityMap[alloyedDenom]

	// Note: we do not update the total liquidity since this is
	// a contribution of the "minting" LP share pool.
	// We only update the total liquidity for an alloyed
	// whenever it is included in the balances as part of the "non-minting" pools.
	// However, for the purposes of finding candidate routes, we treate as if the LP
	// share is contributing zero to the liquidity of the pool it is minted from.
	// See `docs/architecture/COSMWASM_POOLS.md` for details.
	if ok {
		denomPoolLiquidityData.Pools[poolID] = osmomath.ZeroInt()
		currentBlockLiquidityMap[alloyedDenom] = denomPoolLiquidityData
	} else {
		currentBlockLiquidityMap[alloyedDenom] = domain.DenomPoolLiquidityData{
			// Note: we do not update the total liquidity since it is not present in the balances.
			TotalLiquidity: osmomath.ZeroInt(),
			Pools: map[uint64]osmomath.Int{
				poolID: osmomath.ZeroInt(),
			},
		}
	}

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

	// Process the SQS model
	if err := processSQSModelMut(&poolWrapper.SQSModel); err != nil {
		p.logger.Error("error processing SQS model", zap.Error(err))
	}

	return &poolWrapper, nil
}

// executeEndBlockProcessPlugins executes the end block process plugins.
func (p *ingestUseCase) executeEndBlockProcessPlugins(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) {
	for _, plugin := range p.endBlockProcessPlugins {
		if err := plugin.ProcessEndBlock(ctx, blockHeight, metadata); err != nil {
			p.logger.Error("error executing end block process plugin", zap.Error(err), zap.Uint64("block_height", blockHeight))
		}
	}
}

// processSQSModelMut processes the SQS model and updates it.
// Specifically, it removes the gamm shares from the balances and pool denoms.
// Additionally it updates the alloyed denom if it is an alloy transmuter.
func processSQSModelMut(sqsModel *sqsdomain.SQSPool) error {
	// Update alloyed denom since it is not in the balances.
	cosmWasmModel := sqsModel.CosmWasmPoolModel
	if cosmWasmModel != nil && cosmWasmModel.IsAlloyTransmuter() {
		if cosmWasmModel.Data.AlloyTransmuter == nil {
			return fmt.Errorf("alloy transmuter data is nil, skipping silently, contract %s, version %s", cosmWasmModel.ContractInfo.Contract, cosmWasmModel.ContractInfo.Version)
		}

		alloyedDenom := cosmWasmModel.Data.AlloyTransmuter.AlloyedDenom

		sqsModel.PoolDenoms = append(sqsModel.PoolDenoms, alloyedDenom)

		// Process the alloyed pool
		if err := processAlloyedPool(sqsModel); err != nil {
			return err
		}
	}

	// Remove gamm shares from balances
	newBalances := make([]sdk.Coin, 0, len(sqsModel.Balances))

	balancesMap := make(map[string]osmomath.Int)
	for i, balance := range sqsModel.Balances {
		if balance.Validate() != nil {
			continue
		}

		if strings.HasPrefix(balance.Denom, domain.GAMMSharePrefix) {
			continue
		}

		newBalances = append(newBalances, sqsModel.Balances[i])

		balancesMap[balance.Denom] = balance.Amount
	}

	sqsModel.Balances = newBalances

	// Remove gamm shares from pool denoms
	newPoolDenoms := make([]string, 0, len(sqsModel.PoolDenoms))
	for _, denom := range sqsModel.PoolDenoms {
		if strings.HasPrefix(denom, domain.GAMMSharePrefix) {
			continue
		}

		newPoolDenoms = append(newPoolDenoms, denom)
	}

	// Sort the pool denoms by balance amount
	// This is useful for edge case handling for certain pools such as alloyed
	// where the token amounts might get imbalanced, making the liquidity of one denom completely zero.
	// In that case, we would like to deprioritize the out-of-liquidity denoms.
	sort.Slice(newPoolDenoms, func(i, j int) bool {
		amountI, ok := balancesMap[sqsModel.PoolDenoms[i]]
		if !ok {
			return false
		}
		amountJ, ok := balancesMap[sqsModel.PoolDenoms[j]]
		if !ok {
			return true
		}
		return amountI.GTE(amountJ)
	})

	sqsModel.PoolDenoms = newPoolDenoms

	return nil
}
