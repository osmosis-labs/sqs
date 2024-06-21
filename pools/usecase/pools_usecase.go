package usecase

import (
	"context"
	"fmt"
	"sync"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

type poolsUseCase struct {
	pools            sync.Map
	routerRepository routerrepo.RouterRepository
	cosmWasmConfig   domain.CosmWasmPoolRouterConfig

	scalingFactorGetterCb domain.ScalingFactorGetterCb
}

var _ mvc.PoolsUsecase = &poolsUseCase{}

// NewPoolsUsecase will create a new pools use case object
func NewPoolsUsecase(poolsConfig *domain.PoolsConfig, nodeURI string, routerRepository routerrepo.RouterRepository, scalingFactorGetterCb domain.ScalingFactorGetterCb) mvc.PoolsUsecase {
	transmuterCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.TransmuterCodeIDs))
	for _, codeId := range poolsConfig.TransmuterCodeIDs {
		transmuterCodeIDsMap[codeId] = struct{}{}
	}

	alloyedTransmuterCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.AlloyedTransmuterCodeIDs))
	for _, codeId := range poolsConfig.AlloyedTransmuterCodeIDs {
		alloyedTransmuterCodeIDsMap[codeId] = struct{}{}
	}

	generalizedCosmWasmCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.GeneralCosmWasmCodeIDs))
	for _, codeId := range poolsConfig.GeneralCosmWasmCodeIDs {
		generalizedCosmWasmCodeIDsMap[codeId] = struct{}{}
	}

	return &poolsUseCase{
		cosmWasmConfig: domain.CosmWasmPoolRouterConfig{
			TransmuterCodeIDs:        transmuterCodeIDsMap,
			AlloyedTransmuterCodeIDs: alloyedTransmuterCodeIDsMap,
			GeneralCosmWasmCodeIDs:   generalizedCosmWasmCodeIDsMap,
			NodeURI:                  nodeURI,
		},

		pools:                 sync.Map{},
		routerRepository:      routerRepository,
		scalingFactorGetterCb: scalingFactorGetterCb,
	}
}

// GetAllPools returns all pools from the repository.
func (p *poolsUseCase) GetAllPools() (pools []sqsdomain.PoolI, err error) {
	p.pools.Range(func(key, value interface{}) bool {
		pool, ok := value.(sqsdomain.PoolI)
		if !ok {
			err = fmt.Errorf("failed to cast pool with value %v", value)
			return false
		}

		pools = append(pools, pool)
		return true
	})

	return pools, nil
}

// GetRoutesFromCandidates implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetRoutesFromCandidates(candidateRoutes sqsdomain.CandidateRoutes, tokenInDenom, tokenOutDenom string) ([]route.RouteImpl, error) {
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

		// For fault tolerance, instead of bubbling up the error and skipping an entire
		// request, we should detect the error and skip the route.
		skipErrorRoute := false

		for _, candidatePool := range candidateRoute.Pools {
			pool, err := p.GetPool(candidatePool.ID)
			if err != nil {
				return nil, err
			}

			// Get taker fee
			takerFee, exists := p.routerRepository.GetTakerFee(previousTokenOutDenom, candidatePool.TokenOutDenom)
			if !exists {
				takerFee = sqsdomain.DefaultTakerFee
			}

			routablePool, err := pools.NewRoutablePool(pool, candidatePool.TokenOutDenom, takerFee, p.cosmWasmConfig, p.scalingFactorGetterCb)
			if err != nil {
				skipErrorRoute = true
				break
			}

			isGeneralizedCosmWasmPool := routablePool.IsGeneralizedCosmWasmPool()
			if isGeneralizedCosmWasmPool {
				containsGeneralizedCosmWasmPool = true
			}

			// Create routable pool
			routablePools = append(routablePools, routablePool)
		}

		// Skip the route if there was an error
		if skipErrorRoute {
			continue
		}

		routes = append(routes, route.RouteImpl{
			Pools:                      routablePools,
			HasGeneralizedCosmWasmPool: containsGeneralizedCosmWasmPool,
		})
	}

	return routes, nil
}

// GetTickModelMap implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetTickModelMap(poolIDs []uint64) (map[uint64]*sqsdomain.TickModel, error) {
	tickModelMap := make(map[uint64]*sqsdomain.TickModel, len(poolIDs))
	for _, poolID := range poolIDs {
		pool, err := p.GetPool(poolID)
		if err != nil {
			return nil, err
		}

		if pool.GetType() != poolmanagertypes.Concentrated {
			return nil, fmt.Errorf("pool with ID %d is not concentrated", poolID)
		}

		poolWrapper, ok := pool.(*sqsdomain.PoolWrapper)
		if !ok {
			return nil, domain.ConcentratedTickModelNotSetError{
				PoolId: poolID,
			}
		}

		tickModelMap[poolID] = poolWrapper.TickModel
	}

	return tickModelMap, nil
}

// GetPool implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetPool(poolID uint64) (sqsdomain.PoolI, error) {
	poolObj, ok := p.pools.Load(poolID)
	if !ok {
		return nil, domain.PoolNotFoundError{PoolID: poolID}
	}

	pool, ok := poolObj.(sqsdomain.PoolI)
	if !ok {
		return nil, fmt.Errorf("failed to cast pool with ID %d", poolID)
	}

	return pool, nil
}

// GetPoolSpotPrice implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetPoolSpotPrice(ctx context.Context, poolID uint64, takerFee math.LegacyDec, quoteAsset, baseAsset string) (osmomath.BigDec, error) {
	pool, err := p.GetPool(poolID)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Instrument pool with tick model data if concentrated
	if err := p.getTicksAndSetTickModelIfConcentrated(pool); err != nil {
		return osmomath.BigDec{}, err
	}

	// N.B.: Empty string for token out denom because it is irrelevant for calculating spot price.
	// It is only relevant in the context of routing
	routablePool, err := pools.NewRoutablePool(pool, "", takerFee, p.cosmWasmConfig, p.scalingFactorGetterCb)
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
func setTickModelIfConcentrated(pool sqsdomain.PoolI, tickModelMap map[uint64]*sqsdomain.TickModel) error {
	if pool.GetType() == poolmanagertypes.Concentrated {
		// Get tick model for concentrated pool
		tickModel, ok := tickModelMap[pool.GetId()]
		if !ok {
			return domain.ConcentratedTickModelNotSetError{
				PoolId: pool.GetId(),
			}
		}

		if err := pool.SetTickModel(tickModel); err != nil {
			return err
		}
	}

	return nil
}

// getTicksAndSetTickModelIfConcentrated gets tick model for concentrated pools and sets it if this is a concentrated pool.
// The input pool parameter is mutated.
// No-op if pool is not concentrated.
func (p *poolsUseCase) getTicksAndSetTickModelIfConcentrated(pool sqsdomain.PoolI) error {
	if pool.GetType() == poolmanagertypes.Concentrated {
		// Get tick model for concentrated pools
		tickModelMap, err := p.GetTickModelMap([]uint64{pool.GetId()})
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
func (p *poolsUseCase) GetPools(poolIDs []uint64) ([]sqsdomain.PoolI, error) {
	pools := make([]sqsdomain.PoolI, 0, len(poolIDs))

	for _, poolID := range poolIDs {
		pool, err := p.GetPool(poolID)
		if err != nil {
			return nil, err
		}

		pools = append(pools, pool)
	}

	return pools, nil
}

// StorePools implements mvc.PoolsUsecase.
func (p *poolsUseCase) StorePools(pools []sqsdomain.PoolI) error {
	for _, pool := range pools {
		p.pools.Store(pool.GetId(), pool)
	}
	return nil
}

// GetCosmWasmPoolConfig implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetCosmWasmPoolConfig() domain.CosmWasmPoolRouterConfig {
	return p.cosmWasmConfig
}
