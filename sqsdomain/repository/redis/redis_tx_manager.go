package redisrepo

import (
	"github.com/osmosis-labs/sqsdomain/repository"
	"github.com/redis/go-redis/v9"
)

// RedisTxManager is a structure encapsulating creation of atomic transactions.
type RedisTxManager struct {
	client *redis.Client
}

var (
	_ repository.TxManager = &RedisTxManager{}
)

// NewTxManager creates a new TxManager.
func NewTxManager(redisClient *redis.Client) repository.TxManager {
	return &RedisTxManager{
		client: redisClient,
	}
}

// StartTx implements mvc.AtomicRepositoryManager.
func (rm *RedisTxManager) StartTx() repository.Tx {
	return repository.NewRedisTx(rm.client.TxPipeline())
}
