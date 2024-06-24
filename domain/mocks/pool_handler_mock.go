package mocks

import (
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
func (p *PoolHandlerMock) GetPools(poolIDs []uint64) ([]sqsdomain.PoolI, error) {
	if p.ForceGetPoolsError != nil {
		return nil, p.ForceGetPoolsError
	}

	result := make([]sqsdomain.PoolI, 0, len(poolIDs))

	for _, pool := range p.Pools {
		for _, id := range poolIDs {
			if pool.GetId() == id {
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
