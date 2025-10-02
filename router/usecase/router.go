package usecase

import (
	"sort"
	"strings"

	cosmwasmpooltypes "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/types"
	"github.com/osmosis-labs/sqs/domain"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"go.uber.org/zap"

	"github.com/osmosis-labs/sqs/log"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

type ratedPool struct {
	pool   ingesttypes.PoolI
	rating float64
}

const (
	// Pool ordering constants below:

	noPoolLiquidityCapError = ""
)

// filterPoolsByMinLiquidity filters the given pools by the minimum liquidity
// capitalization.
func FilterPoolsByMinLiquidity(pools []ingesttypes.PoolI, minPoolLiquidityCap uint64) []ingesttypes.PoolI {
	minLiquidityCapInt := osmomath.NewIntFromUint64(minPoolLiquidityCap)
	filteredPools := make([]ingesttypes.PoolI, 0, len(pools))
	for _, pool := range pools {
		if pool.GetPoolLiquidityCap().GTE(minLiquidityCapInt) {
			filteredPools = append(filteredPools, pool)
		}
	}
	return filteredPools
}

// ValidateAndSortPools filters and sorts the given pools for use in the router
// according to the given configuration.
// Filters out pools that have no tvl error set and have zero liquidity.
// As a second return value, it returns the orderbook pools.
func ValidateAndSortPools(pools []ingesttypes.PoolI, cosmWasmPoolsConfig domain.CosmWasmPoolRouterConfig, preferredPoolIDs []uint64, logger log.Logger) ([]ingesttypes.PoolI, []ingesttypes.PoolI) {
	filteredPools := make([]ingesttypes.PoolI, 0, len(pools))

	totalTVL := osmomath.ZeroInt()

	orderbookPools := make([]ingesttypes.PoolI, 0)

	// Make a copy and filter pools
	for _, pool := range pools {
		// TODO: the zero argument can be removed in a future release
		// since we will be filtering at a different layer of abstraction.
		if err := pool.Validate(zero); err != nil {
			logger.Debug("pool validation failed, skip silently", zap.Uint64("pool_id", pool.GetId()), zap.Error(err))
			continue
		}

		// Confirm that a cosmwasm code ID is whitelisted via config.
		if pool.GetType() == poolmanagertypes.CosmWasm {
			cosmWasmPool, ok := pool.GetUnderlyingPool().(cosmwasmpooltypes.CosmWasmExtension)
			if !ok {
				logger.Debug("failed to cast a cosm wasm pool, skip silently", zap.Uint64("pool_id", pool.GetId()))
				continue
			}

			_, isTransmuterCodeID := cosmWasmPoolsConfig.TransmuterCodeIDs[cosmWasmPool.GetCodeId()]
			_, isAlloyedTransmuterCodeID := cosmWasmPoolsConfig.AlloyedTransmuterCodeIDs[cosmWasmPool.GetCodeId()]
			_, isOrderbookCodeID := cosmWasmPoolsConfig.OrderbookCodeIDs[cosmWasmPool.GetCodeId()]
			_, isGeneralCosmWasmCodeID := cosmWasmPoolsConfig.GeneralCosmWasmCodeIDs[cosmWasmPool.GetCodeId()]

			if !(isTransmuterCodeID || isAlloyedTransmuterCodeID || isOrderbookCodeID || isGeneralCosmWasmCodeID) {
				logger.Debug("cw pool code id is not added to config, skip silently", zap.Uint64("pool_id", pool.GetId()))

				continue
			}

			if isOrderbookCodeID {
				orderbookPools = append(orderbookPools, pool)
			}
		}

		filteredPools = append(filteredPools, pool)

		totalTVL = totalTVL.Add(pool.GetPoolLiquidityCap())
	}

	preferredPoolIDsMap := make(map[uint64]struct{})
	for _, poolID := range preferredPoolIDs {
		preferredPoolIDsMap[poolID] = struct{}{}
	}

	logger.Debug("validated pools", zap.Int("num_pools", len(filteredPools)))

	return sortPools(filteredPools, cosmWasmPoolsConfig.TransmuterCodeIDs, totalTVL, preferredPoolIDsMap, logger), orderbookPools
}

// sortPools sorts the given pools so that the most appropriate pools are at the top.
// The details of the sorting follow. Assign a rating to each pool based on the following criteria:
// - Initial rating equals to the pool's total value locked denominated in OSMO.
// - If the pool has no error in TVL, add 1/100 of total value locked across all pools to the rating.
// - If the pool is a preferred pool, add the total value locked across all pools to the rating.
// - If the pool is a concentrated pool, add 1/2 of total value locked across all pools to the rating.
// - If the pool is a transmuter pool, add 3/2 of total value locked across all pools to the rating.
// - Sort all pools by the rating score.
//
// This sorting exists to pursue the following heuristics:
// - The TVL is the main metric to sort pools by.
// - Preferred pools are prioritized by getting a boost.
// - Transmuter pools are the most efficient due to no slippage swaps so they get a boost.
// - Simillarly, alloyed transmuter pools are comparable due to no slippage swaps so they get a boost.
// - Concentrated pools follow so they get a smaller boost.
// - Pools with no error in TVL are prioritized by getting an even smaller boost.
//
// These heuristics are imperfect and subject to change.
func sortPools(pools []ingesttypes.PoolI, transmuterCodeIDs map[uint64]struct{}, totalTVL osmomath.Int, preferredPoolIDsMap map[uint64]struct{}, logger log.Logger) []ingesttypes.PoolI {
	logger.Debug("total tvl", zap.Stringer("total_tvl", totalTVL))
	totalTVLFloat, _ := totalTVL.BigIntMut().Float64()

	ratedPools := make([]ratedPool, 0, len(pools))
	for _, pool := range pools {
		// Check if this is a transmuter pool first
		isTransmuterPool := false
		isOrderbookPool := false

		if pool.GetType() == poolmanagertypes.CosmWasm {
			cosmWasmPoolModel := pool.GetSQSPoolModel().CosmWasmPoolModel
			if cosmWasmPoolModel != nil {
				if cosmWasmPoolModel.IsAlloyTransmuter() {
					isTransmuterPool = true
				} else if cosmWasmPoolModel.IsOrderbook() {
					isOrderbookPool = true
				}
			} else {
				cosmWasmPool, ok := pool.GetUnderlyingPool().(cosmwasmpooltypes.CosmWasmExtension)
				if ok {
					_, isTransmuter := transmuterCodeIDs[cosmWasmPool.GetCodeId()]
					if isTransmuter {
						isTransmuterPool = true
					}
				}
			}
		}

		// Initialize rating to TVL.
		rating, _ := pool.GetPoolLiquidityCap().BigIntMut().Float64()

		// rating += 1/ 100 of TVL of asset across all pools
		// (Ignoring any pool with an error in TVL)
		if strings.TrimSpace(pool.GetSQSPoolModel().PoolLiquidityCapError) == noPoolLiquidityCapError {
			rating += totalTVLFloat / 100
		}

		// Preferred pools get a boost equal to the total value locked across all pools
		_, isPreferred := preferredPoolIDsMap[pool.GetId()]
		if isPreferred {
			rating += totalTVLFloat
		}

		// Concentrated pools get a boost equal to 1/2 of total value locked across all pools
		isConcentrated := pool.GetType() == poolmanagertypes.Concentrated
		if isConcentrated {
			rating += totalTVLFloat / 2
		}

		// Orderbook pools get a boost equal to 2x of total value locked across all pools
		if isOrderbookPool {
			rating += totalTVLFloat * 2
		}

		// For transmuter pools, ensure they're always prioritized by adding a massive boost
		// that guarantees higher rating than any non-transmuter pool
		if isTransmuterPool {
			// Add 10x totalTVL to ensure transmuters always rank highest
			// This is added on top of existing rating, so even if pool has zero liquidity cap,
			// it will have rating of 10x totalTVL, which beats any non-transmuter
			rating += totalTVLFloat * 10.0
		}

		ratedPools = append(ratedPools, ratedPool{
			pool:   pool,
			rating: rating,
		})
	}

	// Sort all pools by the rating score
	sort.Slice(ratedPools, func(i, j int) bool {
		return ratedPools[i].rating > ratedPools[j].rating
	})

	logger.Debug("sorted pools", zap.Int("pool_count", len(ratedPools)))
	// Convert back to pools
	for i, ratedPool := range ratedPools {
		pool := ratedPool.pool

		sqsModel := pool.GetSQSPoolModel()
		logger.Debug("pool", zap.Int("index", i), zap.Any("pool", pool.GetId()), zap.Float64("rate", ratedPool.rating), zap.Stringer("pool_liquidity_cap", sqsModel.PoolLiquidityCap), zap.String("pool_liquidity_cap_error", sqsModel.PoolLiquidityCapError))
		pools[i] = ratedPool.pool
	}
	return pools
}
