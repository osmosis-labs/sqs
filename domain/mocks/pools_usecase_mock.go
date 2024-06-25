package mocks

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
)

type PoolsUsecaseMock struct {
	Pools        []sqsdomain.PoolI
	TickModelMap map[uint64]*sqsdomain.TickModel
}

// StorePools implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) StorePools(pools []sqsdomain.PoolI) error {
	panic("unimplemented")
}

// GetCosmWasmPoolConfig implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetCosmWasmPoolConfig() domain.CosmWasmPoolRouterConfig {
	return domain.CosmWasmPoolRouterConfig{
		TransmuterCodeIDs:      map[uint64]struct{}{},
		GeneralCosmWasmCodeIDs: map[uint64]struct{}{},
		NodeURI:                "",
	}
}

// GetPools implements mvc.PoolsUsecase.
func (*PoolsUsecaseMock) GetPools(poolIDs []uint64) ([]sqsdomain.PoolI, error) {
	panic("unimplemented")
}

// GetRoutesFromCandidates implements mvc.PoolsUsecase.
// Note that taker fee are ignored and not set
// Note that tick models are not set
func (pm *PoolsUsecaseMock) GetRoutesFromCandidates(candidateRoutes sqsdomain.CandidateRoutes, tokenInDenom string, tokenOutDenom string) ([]route.RouteImpl, error) {
	finalRoutes := make([]route.RouteImpl, 0, len(candidateRoutes.Routes))
	for _, candidateRoute := range candidateRoutes.Routes {
		routablePools := make([]domain.RoutablePool, 0, len(candidateRoute.Pools))
		for _, candidatePool := range candidateRoute.Pools {
			// Get the pool data for routing
			var foundPool sqsdomain.PoolI
			for _, pool := range pm.Pools {
				if pool.GetId() == candidatePool.ID {
					foundPool = pool
				}
			}

			if foundPool == nil {
				return nil, fmt.Errorf("pool with id %d not found in pools use case mock", candidatePool.ID)
			}

			// TODO: note that taker fee is force set to zero
			routablePool, err := pools.NewRoutablePool(foundPool, candidatePool.TokenOutDenom, osmomath.ZeroDec(), domain.CosmWasmPoolRouterConfig{}, domain.UnsetScalingFactorGetterCb)
			if err != nil {
				return nil, err
			}
			routablePools = append(routablePools, routablePool)
		}

		finalRoutes = append(finalRoutes, route.RouteImpl{
			Pools: routablePools,
		})
	}

	return finalRoutes, nil
}

// GetAllPools implements domain.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetAllPools() ([]sqsdomain.PoolI, error) {
	return pm.Pools, nil
}

// GetTickModelMap implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetTickModelMap(poolIDs []uint64) (map[uint64]*sqsdomain.TickModel, error) {
	return pm.TickModelMap, nil
}

// GetPool implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetPool(poolID uint64) (sqsdomain.PoolI, error) {
	panic("unimplemented")
}

// GetPoolSpotPrice implements mvc.PoolsUsecase.
func (*PoolsUsecaseMock) GetPoolSpotPrice(ctx context.Context, poolID uint64, takerFee math.LegacyDec, baseAsset, quoteAsset string) (osmomath.BigDec, error) {
	panic("unimplemented")
}

var _ mvc.PoolsUsecase = &PoolsUsecaseMock{}
