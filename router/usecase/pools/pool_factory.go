package pools

import (
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/osmomath"
	concentratedmodel "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/model"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v25/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v25/x/gamm/pool-models/stableswap"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

// NewRoutablePool creates a new RoutablePool.
// Panics if pool is of invalid type or if does not contain tick data when a concentrated pool.
func NewRoutablePool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec, cosmWasmConfig domain.CosmWasmPoolRouterConfig) (sqsdomain.RoutablePool, error) {
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

		// Check if pools is balancer
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

	return newRoutableCosmWasmPool(pool, cosmWasmConfig, tokenOutDenom, takerFee)
}

// newRoutableCosmWasmPool creates a new RoutablePool for CosmWasm pools.
// Panics if the given pool is not a cosmwasm pool or if the
func newRoutableCosmWasmPool(pool sqsdomain.PoolI, cosmWasmConfig domain.CosmWasmPoolRouterConfig, tokenOutDenom string, takerFee osmomath.Dec) (sqsdomain.RoutablePool, error) {
	chainPool := pool.GetUnderlyingPool()
	poolType := pool.GetType()

	cosmwasmPool, ok := chainPool.(*cwpoolmodel.CosmWasmPool)
	if !ok {
		return nil, domain.FailedToCastPoolModelError{
			ExpectedModel: poolmanagertypes.PoolType_name[int32(poolmanagertypes.CosmWasm)],
			ActualModel:   poolmanagertypes.PoolType_name[int32(poolType)],
		}
	}

	// Check if the pool is a transmuter pool
	_, isTransmuter := cosmWasmConfig.TransmuterCodeIDs[cosmwasmPool.CodeId]
	if isTransmuter {
		spreadFactor := pool.GetSQSPoolModel().SpreadFactor

		// Transmuter has a custom implementation since it does not need to interact with the chain.
		return &routableTransmuterPoolImpl{
			ChainPool:     cosmwasmPool,
			Balances:      pool.GetSQSPoolModel().Balances,
			TokenOutDenom: tokenOutDenom,
			TakerFee:      takerFee,
			SpreadFactor:  spreadFactor,
		}, nil
	}

	_, isGeneralizedCosmWasmPool := cosmWasmConfig.GeneralCosmWasmCodeIDs[cosmwasmPool.CodeId]
	if isGeneralizedCosmWasmPool {
		wasmClient, err := initializeWasmClient(cosmWasmConfig.NodeURI)
		if err != nil {
			return nil, err
		}

		spreadFactor := pool.GetSQSPoolModel().SpreadFactor

		// for most other cosm wasm pools, interaction with the chain will
		// be required. As a result, we have a custom implementation.
		return &routableCosmWasmPoolImpl{
			ChainPool:     cosmwasmPool,
			Balances:      pool.GetSQSPoolModel().Balances,
			TokenOutDenom: tokenOutDenom,
			TakerFee:      takerFee,
			SpreadFactor:  spreadFactor,
			wasmClient:    wasmClient,
		}, nil
	}

	return nil, domain.UnsupportedCosmWasmPoolTypeError{
		PoolType: poolmanagertypes.PoolType_name[int32(poolType)],
		PoolId:   cosmwasmPool.PoolId,
	}
}
