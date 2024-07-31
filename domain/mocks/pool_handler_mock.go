package mocks

import (
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type PoolHandlerMock struct {
	Pools                []sqsdomain.PoolI
	ForceGetPoolsError   error
	ForceStorePoolsError error
}

var _ mvc.PoolHandler = &PoolHandlerMock{}

// GetPools implements mvc.PoolHandler.
func (p *PoolHandlerMock) GetPools(opts ...domain.PoolsOption) ([]sqsdomain.PoolI, error) {
	if p.ForceGetPoolsError != nil {
		return nil, p.ForceGetPoolsError
	}

	options := domain.PoolsOptions{
		MinPoolLiquidityCap: 0,
		PoolIDFilter:        []uint64{},
	}

	for _, opt := range opts {
		opt(&options)
	}

	result := make([]sqsdomain.PoolI, 0)

	if len(options.PoolIDFilter) > 0 {
		for _, id := range options.PoolIDFilter {
			for _, pool := range p.Pools {
				if pool.GetId() == id {
					result = append(result, pool)
				}
			}
		}
	} else {
		for _, pool := range p.Pools {
			if pool.GetLiquidityCap().Uint64() > options.MinPoolLiquidityCap {
				result = append(result, pool)
			}
		}
	}

	return result, nil
}

// StorePools implements mvc.PoolHandler.
func (p *PoolHandlerMock) StorePools(pools []sqsdomain.PoolI) error {
	if p.ForceStorePoolsError != nil {
		return p.ForceStorePoolsError
	}

	for _, updatedPool := range pools {
		// By default, if the updated pool did not exist already in the mock
		// we append it.
		shouldAppend := true

		for i, existingPool := range p.Pools {
			// If pool already existed, update it.
			if existingPool.GetId() == updatedPool.GetId() {
				p.Pools[i] = updatedPool

				shouldAppend = false

				break
			}
		}

		if shouldAppend {
			p.Pools = append(p.Pools, updatedPool)
		}
	}
	return nil
}

// CalcExitCFMMPool implements mvc.PoolHandler.
func (p *PoolHandlerMock) CalcExitCFMMPool(poolID uint64, exitingShares math.Int) (types.Coins, error) {
	panic("unimplemented")
}
