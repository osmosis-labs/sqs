package usecase

import (
	"context"
	"time"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v22/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/sqsdomain/repository"
	poolsredisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis/pools"
)

type poolsUseCase struct {
	contextTimeout         time.Duration
	poolsRepository        poolsredisrepo.PoolsRepository
	redisRepositoryManager repository.TxManager
	cosmWasmConfig         domain.CosmWasmPoolRouterConfig
}

var _ mvc.PoolsUsecase = &poolsUseCase{}

// NewPoolsUsecase will create a new pools use case object
func NewPoolsUsecase(timeout time.Duration, poolsRepository poolsredisrepo.PoolsRepository, redisRepositoryManager repository.TxManager, poolsConfig *domain.PoolsConfig, nodeURI string) mvc.PoolsUsecase {
	transmuterCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.TransmuterCodeIDs))
	for _, codeId := range poolsConfig.TransmuterCodeIDs {
		transmuterCodeIDsMap[codeId] = struct{}{}
	}

	generalizedCosmWasmCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.GeneralCosmWasmCodeIDs))
	for _, codeId := range poolsConfig.GeneralCosmWasmCodeIDs {
		generalizedCosmWasmCodeIDsMap[codeId] = struct{}{}
	}

	return &poolsUseCase{
		contextTimeout:         timeout,
		poolsRepository:        poolsRepository,
		redisRepositoryManager: redisRepositoryManager,
		cosmWasmConfig: domain.CosmWasmPoolRouterConfig{
			TransmuterCodeIDs:      transmuterCodeIDsMap,
			GeneralCosmWasmCodeIDs: generalizedCosmWasmCodeIDsMap,
			NodeURI:                nodeURI,
		},
	}
}

// GetAllPools returns all pools from the repository.
func (p *poolsUseCase) GetAllPools(ctx context.Context) ([]sqsdomain.PoolI, error) {
	ctx, cancel := context.WithTimeout(ctx, p.contextTimeout)
	defer cancel()

	pools, err := p.poolsRepository.GetAllPools(ctx)
	if err != nil {
		return nil, err
	}

	return pools, nil
}

// GetRoutesFromCandidates implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetRoutesFromCandidates(ctx context.Context, candidateRoutes sqsdomain.CandidateRoutes, takerFeeMap sqsdomain.TakerFeeMap, tokenInDenom, tokenOutDenom string) ([]route.RouteImpl, error) {
	// Get all pools
	poolsData, err := p.poolsRepository.GetPools(ctx, candidateRoutes.UniquePoolIDs)
	if err != nil {
		return nil, err
	}

	// TODO: refactor get these directl from the pools repository.
	// Get conentrated pools and separately get tick model for them
	concentratedPoolIDs := make([]uint64, 0)
	for _, candidatePool := range poolsData {
		if candidatePool.GetType() == poolmanagertypes.Concentrated {
			concentratedPoolIDs = append(concentratedPoolIDs, candidatePool.GetId())
		}
	}

	// Get tick model for concentrated pools
	tickModelMap, err := p.GetTickModelMap(ctx, concentratedPoolIDs)
	if err != nil {
		return nil, err
	}

	// We track whether a route contains a generalized cosmwasm pool
	// so that we can exclude it from split quote logic.
	// The reason for this is that making network requests to chain is expensive.
	// As a result, we want to minimize the number of requests we make.
	containsGeneralizedCosmWasmPool := false

	// Convert each candidate route into the actual route with all pool data
	routes := make([]route.RouteImpl, 0, len(candidateRoutes.Routes))
	for _, candidateRoute := range candidateRoutes.Routes {
		previousTokenOutDenom := tokenInDenom
		routablePools := make([]sqsdomain.RoutablePool, 0, len(candidateRoute.Pools))
		for _, candidatePool := range candidateRoute.Pools {
			// Get the pool data for routing
			pool, ok := poolsData[candidatePool.ID]
			if !ok {
				return nil, domain.PoolNotFoundError{PoolID: candidatePool.ID}
			}

			// Get taker fee
			takerFee := takerFeeMap.GetTakerFee(previousTokenOutDenom, candidatePool.TokenOutDenom)

			// Instrument pool with tick model data if concentrated
			if err := setTickModelIfConcentrated(pool, tickModelMap); err != nil {
				return nil, err
			}

			routablePool, err := pools.NewRoutablePool(pool, candidatePool.TokenOutDenom, takerFee, p.cosmWasmConfig)
			if err != nil {
				return nil, err
			}

			isGeneralizedCosmWasmPool := routablePool.IsGeneralizedCosmWasmPool()
			if isGeneralizedCosmWasmPool {
				containsGeneralizedCosmWasmPool = true
			}

			// Create routable pool
			routablePools = append(routablePools, routablePool)
		}

		routes = append(routes, route.RouteImpl{
			Pools:                      routablePools,
			HasGeneralizedCosmWasmPool: containsGeneralizedCosmWasmPool,
		})
	}

	return routes, nil
}

// GetTickModelMap implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetTickModelMap(ctx context.Context, poolIDs []uint64) (map[uint64]sqsdomain.TickModel, error) {
	ctx, cancel := context.WithTimeout(ctx, p.contextTimeout)
	defer cancel()

	tickModelMap, err := p.poolsRepository.GetTickModelForPools(ctx, poolIDs)
	if err != nil {
		return nil, err
	}

	return tickModelMap, nil
}

// GetPool implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetPool(ctx context.Context, poolID uint64) (sqsdomain.PoolI, error) {
	pools, err := p.poolsRepository.GetPools(ctx, map[uint64]struct {
	}{
		poolID: {},
	})

	if err != nil {
		return nil, err
	}

	pool, ok := pools[poolID]
	if !ok {
		return nil, domain.PoolNotFoundError{PoolID: poolID}
	}
	return pool, nil
}

// GetPoolSpotPrice implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetPoolSpotPrice(ctx context.Context, poolID uint64, takerFee math.LegacyDec, quoteAsset, baseAsset string) (osmomath.BigDec, error) {
	pool, err := p.GetPool(ctx, poolID)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Instrument pool with tick model data if concentrated
	if err := p.getTicksAndSetTickModelIfConcentrated(ctx, pool); err != nil {
		return osmomath.BigDec{}, err
	}

	// N.B.: Empty string for token out denom because it is irrelevant for calculating spot price.
	// It is only relevant in the context of routing
	routablePool, err := pools.NewRoutablePool(pool, "", takerFee, p.cosmWasmConfig)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return routablePool.CalcSpotPrice(ctx, baseAsset, quoteAsset)
}

// IsGeneralCosmWasmCodeID implements mvc.PoolsUsecase.
func (p *poolsUseCase) IsGeneralCosmWasmCodeID(codeId uint64) bool {
	_, isGenneralCosmWasmCodeID := p.cosmWasmConfig.GeneralCosmWasmCodeIDs[codeId]
	return isGenneralCosmWasmCodeID
}

// setTickModelMapIfConcentrated sets tick model for concentrated pools. No-op if pool is not concentrated.
// If the pool is concentrated but the map does not contains the tick model, an error is returned.
// The input pool parameter is mutated.
func setTickModelIfConcentrated(pool sqsdomain.PoolI, tickModelMap map[uint64]sqsdomain.TickModel) error {
	if pool.GetType() == poolmanagertypes.Concentrated {
		// Get tick model for concentrated pool
		tickModel, ok := tickModelMap[pool.GetId()]
		if !ok {
			return domain.ConcentratedTickModelNotSetError{
				PoolId: pool.GetId(),
			}
		}

		if err := pool.SetTickModel(&tickModel); err != nil {
			return err
		}
	}

	return nil
}

// getTicksAndSetTickModelIfConcentrated gets tick model for concentrated pools and sets it if this is a concentrated pool.
// The input pool parameter is mutated.
// No-op if pool is not concentrated.
func (p *poolsUseCase) getTicksAndSetTickModelIfConcentrated(ctx context.Context, pool sqsdomain.PoolI) error {
	if pool.GetType() == poolmanagertypes.Concentrated {
		// Get tick model for concentrated pools
		tickModelMap, err := p.GetTickModelMap(ctx, []uint64{pool.GetId()})
		if err != nil {
			return err
		}

		// Set tick model for concentrated pools
		if err := setTickModelIfConcentrated(pool, tickModelMap); err != nil {
			return err
		}
	}

	return nil
}

// GetPools implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetPools(ctx context.Context, poolIDs []uint64) ([]sqsdomain.PoolI, error) {
	// Convert given IDs to map
	poolIDsMap := make(map[uint64]struct{}, len(poolIDs))
	for _, poolID := range poolIDs {
		poolIDsMap[poolID] = struct{}{}
	}

	// Get pools
	poolsMap, err := p.poolsRepository.GetPools(ctx, poolIDsMap)
	if err != nil {
		return nil, err
	}

	// Convert to slice
	pools := make([]sqsdomain.PoolI, 0, len(poolsMap))
	for _, pool := range poolsMap {
		pools = append(pools, pool)
	}

	return pools, nil
}
