package pools

import (
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/osmomath"
	concentratedmodel "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/model"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/stableswap"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

// NewRoutablePool creates a new RoutablePool.
// Panics if pool is of invalid type or if does not contain tick data when a concentrated pool.
func NewRoutablePool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec, cosmWasmPoolsParams cosmwasmdomain.CosmWasmPoolsParams) (domain.RoutablePool, error) {
	poolType := pool.GetType()
	chainPool := pool.GetUnderlyingPool()
	if poolType == poolmanagertypes.Concentrated {
		// Check if pools is concentrated
		concentratedPool, ok := chainPool.(*concentratedmodel.Pool)
		if !ok {
			panic(domain.FailedToCastPoolModelError{
				ExpectedModel: poolmanagertypes.PoolType_name[int32(poolmanagertypes.Concentrated)],
				ActualModel:   poolmanagertypes.PoolType_name[int32(poolType)],
			})
		}

		tickModel, err := pool.GetTickModel()
		if err != nil {
			panic(err)
		}

		return &routableConcentratedPoolImpl{
			ChainPool:     concentratedPool,
			TickModel:     tickModel,
			TokenOutDenom: tokenOutDenom,
			TakerFee:      takerFee,
		}, nil
	}

	if poolType == poolmanagertypes.Balancer {
		chainPool := pool.GetUnderlyingPool()

		// Check if pools is balancer
		balancerPool, ok := chainPool.(*balancer.Pool)
		if !ok {
			panic(domain.FailedToCastPoolModelError{
				ExpectedModel: poolmanagertypes.PoolType_name[int32(poolmanagertypes.Balancer)],
				ActualModel:   poolmanagertypes.PoolType_name[int32(poolType)],
			})
		}

		return &routableBalancerPoolImpl{
			ChainPool:     balancerPool,
			TokenOutDenom: tokenOutDenom,
			TakerFee:      takerFee,
		}, nil
	}

	if pool.GetType() == poolmanagertypes.Stableswap {
		// Must be stableswap
		if poolType != poolmanagertypes.Stableswap {
			panic(domain.InvalidPoolTypeError{
				PoolType: int32(poolType),
			})
		}

		// Check if pools is stableswap
		stableswapPool, ok := chainPool.(*stableswap.Pool)
		if !ok {
			panic(domain.FailedToCastPoolModelError{
				ExpectedModel: poolmanagertypes.PoolType_name[int32(poolmanagertypes.Stableswap)],
				ActualModel:   poolmanagertypes.PoolType_name[int32(poolType)],
			})
		}

		return &routableStableswapPoolImpl{
			ChainPool:     stableswapPool,
			TokenOutDenom: tokenOutDenom,
			TakerFee:      takerFee,
		}, nil
	}

	return newRoutableCosmWasmPool(pool, tokenOutDenom, takerFee, cosmWasmPoolsParams)
}

// newRoutableCosmWasmPool creates a new RoutablePool for CosmWasm pools.
// Panics if the given pool is not a cosmwasm pool or if the
func newRoutableCosmWasmPool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec, cosmWasmPoolsParams cosmwasmdomain.CosmWasmPoolsParams) (domain.RoutablePool, error) {
	chainPool := pool.GetUnderlyingPool()
	poolType := pool.GetType()

	cosmwasmPool, ok := chainPool.(*cwpoolmodel.CosmWasmPool)
	if !ok {
		return nil, domain.FailedToCastPoolModelError{
			ExpectedModel: poolmanagertypes.PoolType_name[int32(poolmanagertypes.CosmWasm)],
			ActualModel:   poolmanagertypes.PoolType_name[int32(poolType)],
		}
	}

	balances := pool.GetSQSPoolModel().Balances

	// Check if the pool is a transmuter pool
	_, isTransmuter := cosmWasmPoolsParams.Config.TransmuterCodeIDs[cosmwasmPool.CodeId]
	if isTransmuter {
		spreadFactor := pool.GetSQSPoolModel().SpreadFactor

		// Transmuter has a custom implementation since it does not need to interact with the chain.
		return &routableTransmuterPoolImpl{
			ChainPool:     cosmwasmPool,
			Balances:      balances,
			TokenOutDenom: tokenOutDenom,
			TakerFee:      takerFee,
			SpreadFactor:  spreadFactor,
		}, nil
	}

	_, isGeneralizedCosmWasmPool := cosmWasmPoolsParams.Config.GeneralCosmWasmCodeIDs[cosmwasmPool.CodeId]
	if isGeneralizedCosmWasmPool {
		spreadFactor := pool.GetSQSPoolModel().SpreadFactor

		// for most other CosmWasm pools, interaction with the chain will
		// be required. As a result, we have a custom implementation.
		return NewRoutableCosmWasmPool(cosmwasmPool, balances, tokenOutDenom, takerFee, spreadFactor, cosmWasmPoolsParams), nil
	}

	return newRoutableCosmWasmPoolWithCustomModel(pool, cosmwasmPool, cosmWasmPoolsParams, tokenOutDenom, takerFee)
}

// newRoutableCosmWasmPoolWithCustomModel creates a new RoutablePool for CosmWasm pools that require a custom CosmWasmPoolModel.
// errors if:
// - the pool matched criteria for a custom model, but the model does not have the required data.
// - the pool's `CosmWasmPoolModel` is nil
// returns a routable pool constructed with custom model otherwise
func newRoutableCosmWasmPoolWithCustomModel(
	pool sqsdomain.PoolI,
	cosmwasmPool *cwpoolmodel.CosmWasmPool,
	cosmWasmPoolsParams cosmwasmdomain.CosmWasmPoolsParams,
	tokenOutDenom string,
	takerFee osmomath.Dec,
) (domain.RoutablePool, error) {
	sqsPoolModel := pool.GetSQSPoolModel()

	// Check if the pool is a transmuter pool with alloyed assets
	model := sqsPoolModel.CosmWasmPoolModel
	balances := sqsPoolModel.Balances
	spreadFactor := sqsPoolModel.SpreadFactor
	if model != nil {
		// since v2, we introduce concept of alloyed assets but not yet actively used
		// since v3, we introduce concept of normalization factor
		// `routableAlloyTransmuterPoolImpl` is v3 compatible
		_, isAlloyedTransmuterCodeId := cosmWasmPoolsParams.Config.AlloyedTransmuterCodeIDs[cosmwasmPool.CodeId]
		if isAlloyedTransmuterCodeId && model.IsAlloyTransmuter() {
			if model.Data.AlloyTransmuter == nil {
				return nil, domain.CosmWasmPoolDataMissingError{
					CosmWasmPoolType: domain.CosmWasmPoolAlloyTransmuter,
					PoolId:           pool.GetId(),
				}
			}

			return &routableAlloyTransmuterPoolImpl{
				ChainPool:           cosmwasmPool,
				AlloyTransmuterData: model.Data.AlloyTransmuter,
				Balances:            balances,
				TokenOutDenom:       tokenOutDenom,
				TakerFee:            takerFee,
				SpreadFactor:        spreadFactor,
			}, nil
		}

		_, isOrderbookCodeId := cosmWasmPoolsParams.Config.OrderbookCodeIDs[cosmwasmPool.CodeId]
		if isOrderbookCodeId && model.IsOrderbook() {
			if model.Data.Orderbook == nil {
				return nil, domain.CosmWasmPoolDataMissingError{
					CosmWasmPoolType: domain.CosmWasmPoolOrderbook,
					PoolId:           pool.GetId(),
				}
			}

			return &routableOrderbookPoolImpl{
				ChainPool:     cosmwasmPool,
				Balances:      balances,
				TokenOutDenom: tokenOutDenom,
				TakerFee:      takerFee,
				SpreadFactor:  spreadFactor,
				OrderbookData: model.Data.Orderbook,
			}, nil
		}
	}

	return nil, domain.UnsupportedCosmWasmPoolError{
		PoolId: cosmwasmPool.PoolId,
	}
}
