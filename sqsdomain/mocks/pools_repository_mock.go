package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/repository"
	poolsredisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis/pools"
)

type RedisPoolsRepositoryMock struct {
	Pools     []sqsdomain.PoolI
	TickModel map[uint64]*sqsdomain.TickModel
}

// GetPools implements mvc.PoolsRepository.
func (r *RedisPoolsRepositoryMock) GetPools(ctx context.Context, poolIDs map[uint64]struct{}) (map[uint64]sqsdomain.PoolI, error) {
	result := map[uint64]sqsdomain.PoolI{}
	for _, pool := range r.Pools {
		result[pool.GetId()] = pool
	}
	return result, nil
}

// GetTickModelForPools implements mvc.PoolsRepository.
func (r *RedisPoolsRepositoryMock) GetTickModelForPools(ctx context.Context, pools []uint64) (map[uint64]*sqsdomain.TickModel, error) {
	return r.TickModel, nil
}

// ClearAllPools implements domain.PoolsRepository.
func (*RedisPoolsRepositoryMock) ClearAllPools(ctx context.Context, tx repository.Tx) error {
	panic("unimplemented")
}

var _ poolsredisrepo.PoolsRepository = &RedisPoolsRepositoryMock{}

// GetAllPools implements domain.PoolsRepository.
func (r *RedisPoolsRepositoryMock) GetAllPools(context.Context) ([]sqsdomain.PoolI, error) {
	allPools := make([]sqsdomain.PoolI, len(r.Pools))
	copy(allPools, r.Pools)
	return allPools, nil
}

// StorePools implements domain.PoolsRepository.
func (r *RedisPoolsRepositoryMock) StorePools(ctx context.Context, tx repository.Tx, allPools []sqsdomain.PoolI) error {
	r.Pools = allPools
	return nil
}
