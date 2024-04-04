package chaininforepo

import (
	"sync"
)

// ChainInfoRepository represents the contract for a repository handling chain information
type ChainInfoRepository interface {
	// StoreLatestHeight stores the latest blockchain height
	StoreLatestHeight(height uint64)

	// GetLatestHeight retrieves the latest blockchain height
	GetLatestHeight() uint64
}

var _ ChainInfoRepository = &chainInfoRepo{}

type chainInfoRepo struct {
	latestHeight uint64
	mu           sync.RWMutex
}

// New creates a new repository for chain information
func New() ChainInfoRepository {
	return &chainInfoRepo{
		latestHeight: 0,
		mu:           sync.RWMutex{},
	}
}

// StoreLatestHeight stores the latest blockchain height into store
func (r *chainInfoRepo) StoreLatestHeight(height uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.latestHeight = height
}

// GetLatestHeight retrieves the latest blockchain height store.
func (r *chainInfoRepo) GetLatestHeight() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.latestHeight
}
