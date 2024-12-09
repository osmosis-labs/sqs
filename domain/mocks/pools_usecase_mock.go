package mocks

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
)

var _ mvc.PoolsUsecase = &PoolsUsecaseMock{}

type PoolsUsecaseMock struct {
	GetAllPoolsFunc                     func() ([]sqsdomain.PoolI, error)
	GetPoolsFunc                        func(opts ...domain.PoolsOption) ([]sqsdomain.PoolI, uint64, error)
	StorePoolsFunc                      func(pools []sqsdomain.PoolI) error
	GetRoutesFromCandidatesFunc         func(candidateRoutes sqsdomain.CandidateRoutes, tokenInDenom, tokenOutDenom string) ([]route.RouteImpl, error)
	GetTickModelMapFunc                 func(poolIDs []uint64) (map[uint64]*sqsdomain.TickModel, error)
	GetPoolFunc                         func(poolID uint64) (sqsdomain.PoolI, error)
	GetPoolSpotPriceFunc                func(ctx context.Context, poolID uint64, takerFee osmomath.Dec, quoteAsset, baseAsset string) (osmomath.BigDec, error)
	GetCosmWasmPoolConfigFunc           func() domain.CosmWasmPoolRouterConfig
	CalcExitCFMMPoolFunc                func(poolID uint64, exitingShares osmomath.Int) (sdk.Coins, error)
	GetAllCanonicalOrderbookPoolIDsFunc func() ([]domain.CanonicalOrderBooksResult, error)

	Pools        []sqsdomain.PoolI
	TickModelMap map[uint64]*sqsdomain.TickModel
}

// IsCanonicalOrderbookPool implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) IsCanonicalOrderbookPool(poolID uint64) bool {
	panic("unimplemented")
}

// GetAllCanonicalOrderbookPoolIDs implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetAllCanonicalOrderbookPoolIDs() ([]domain.CanonicalOrderBooksResult, error) {
	if pm.GetAllCanonicalOrderbookPoolIDsFunc != nil {
		return pm.GetAllCanonicalOrderbookPoolIDsFunc()
	}
	panic("unimplemented")
}

func (pm *PoolsUsecaseMock) WithGetAllCanonicalOrderbookPoolIDs(result []domain.CanonicalOrderBooksResult, err error) {
	pm.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
		return result, err
	}
}

// GetCanonicalOrderbookPool implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetCanonicalOrderbookPool(baseDenom string, quoteDenom string) (uint64, string, error) {
	panic("unimplemented")
}

// StorePools implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) StorePools(pools []sqsdomain.PoolI) error {
	if pm.StorePoolsFunc != nil {
		return pm.StorePoolsFunc(pools)
	}
	panic("unimplemented")
}

// GetCosmWasmPoolConfig implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetCosmWasmPoolConfig() domain.CosmWasmPoolRouterConfig {
	if pm.GetCosmWasmPoolConfigFunc != nil {
		return pm.GetCosmWasmPoolConfigFunc()
	}
	return domain.CosmWasmPoolRouterConfig{
		TransmuterCodeIDs:        map[uint64]struct{}{},
		GeneralCosmWasmCodeIDs:   map[uint64]struct{}{},
		ChainGRPCGatewayEndpoint: "",
	}
}

// GetPools implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetPools(opts ...domain.PoolsOption) ([]sqsdomain.PoolI, uint64, error) {
	if pm.GetPoolsFunc != nil {
		return pm.GetPoolsFunc(opts...)
	}
	panic("unimplemented")
}

// GetRoutesFromCandidates implements mvc.PoolsUsecase.
// Note that taker fee are ignored and not set
// Note that tick models are not set
func (pm *PoolsUsecaseMock) GetRoutesFromCandidates(candidateRoutes sqsdomain.CandidateRoutes, tokenInDenom string, tokenOutDenom string) ([]route.RouteImpl, error) {
	if pm.GetRoutesFromCandidatesFunc != nil {
		return pm.GetRoutesFromCandidatesFunc(candidateRoutes, tokenInDenom, tokenOutDenom)
	}

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
			routablePool, err := pools.NewRoutablePool(foundPool, candidatePool.TokenOutDenom, osmomath.ZeroDec(), cosmwasmdomain.CosmWasmPoolsParams{
				ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
			})
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
	if pm.GetAllPoolsFunc != nil {
		return pm.GetAllPoolsFunc()
	}
	return pm.Pools, nil
}

// GetTickModelMap implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetTickModelMap(poolIDs []uint64) (map[uint64]*sqsdomain.TickModel, error) {
	if pm.GetTickModelMapFunc != nil {
		return pm.GetTickModelMapFunc(poolIDs)
	}
	return pm.TickModelMap, nil
}

// GetPool implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetPool(poolID uint64) (sqsdomain.PoolI, error) {
	if pm.GetPoolFunc != nil {
		return pm.GetPoolFunc(poolID)
	}
	panic("unimplemented")
}

// GetPoolSpotPrice implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) GetPoolSpotPrice(ctx context.Context, poolID uint64, takerFee math.LegacyDec, baseAsset, quoteAsset string) (osmomath.BigDec, error) {
	if pm.GetPoolSpotPriceFunc != nil {
		return pm.GetPoolSpotPriceFunc(ctx, poolID, takerFee, quoteAsset, baseAsset)
	}
	panic("unimplemented")
}

// CalcExitCFMMPool implements mvc.PoolsUsecase.
func (pm *PoolsUsecaseMock) CalcExitCFMMPool(poolID uint64, exitingShares osmomath.Int) (sdk.Coins, error) {
	if pm.CalcExitCFMMPoolFunc != nil {
		return pm.CalcExitCFMMPoolFunc(poolID, exitingShares)
	}
	panic("unimplemented")
}

var _ mvc.PoolsUsecase = &PoolsUsecaseMock{}
