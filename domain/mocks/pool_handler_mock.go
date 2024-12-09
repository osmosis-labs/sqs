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
func (p *PoolHandlerMock) GetPools(opts ...domain.PoolsOption) ([]sqsdomain.PoolI, uint64, error) {
	if p.ForceGetPoolsError != nil {
		return nil, 0, p.ForceGetPoolsError
	}

	var options domain.PoolsOptions
	for _, opt := range opts {
		opt(&options)
	}

	result := make([]sqsdomain.PoolI, 0)
	if f := options.Filter; f != nil && len(f.PoolId) > 0 {
		for _, id := range f.PoolId {
			for _, pool := range p.Pools {
				if pool.GetId() == id {
					result = append(result, pool)
				}
			}
		}
	} else {
		for _, pool := range p.Pools {
			if f := options.Filter; f != nil {
				if pool.GetLiquidityCap().Uint64() > f.MinLiquidityCap {
					result = append(result, pool)
				}
			}
		}
	}

	return result, uint64(len(result)), nil
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
