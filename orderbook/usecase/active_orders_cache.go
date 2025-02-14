package orderbookusecase

import (
	"fmt"
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

// cacheKey represents the composite key for the active orders cache
type cacheKey struct {
	poolID      uint64
	userAddress string
}

// activeOrdersCacheEntry represents a single cache entry containing active orders and a best-effort flag
type activeOrdersCacheEntry struct {
	Orders       []orderbookdomain.LimitOrder
	IsBestEffort bool
}

// activeOrdersCache is a thread-safe LRU cache for active orders
type activeOrdersCache struct {
	cache *lru.Cache[cacheKey, activeOrdersCacheEntry]
	mu    sync.RWMutex
	// poolEntries tracks which cache keys belong to which pool for bulk invalidation
	poolEntries map[uint64]map[string]struct{}
}

// newActiveOrdersCache creates a new active orders cache with the specified size
func newActiveOrdersCache(size int) (*activeOrdersCache, error) {
	cache, err := lru.New[cacheKey, activeOrdersCacheEntry](size)
	if err != nil {
		return nil, fmt.Errorf("failed to create LRU cache: %w", err)
	}

	return &activeOrdersCache{
		cache:       cache,
		poolEntries: make(map[uint64]map[string]struct{}),
	}, nil
}

// get retrieves active orders for a given pool ID and user address
func (c *activeOrdersCache) get(poolID uint64, userAddress string) (activeOrdersCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := cacheKey{poolID: poolID, userAddress: userAddress}
	return c.cache.Get(key)
}

// set stores active orders for a given pool ID and user address
func (c *activeOrdersCache) set(poolID uint64, userAddress string, entry activeOrdersCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey{poolID: poolID, userAddress: userAddress}

	// Track this key for the pool
	if _, exists := c.poolEntries[poolID]; !exists {
		c.poolEntries[poolID] = make(map[string]struct{})
	}
	c.poolEntries[poolID][userAddress] = struct{}{}

	c.cache.Add(key, entry)
}

// invalidatePool removes all cached entries for a given pool ID
func (c *activeOrdersCache) invalidatePool(poolID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get all keys for this pool
	if keys, exists := c.poolEntries[poolID]; exists {
		// Remove each key from the cache
		for userAddress := range keys {
			key := cacheKey{poolID: poolID, userAddress: userAddress}
			c.cache.Remove(key)
		}
		// Remove the pool entry tracking
		delete(c.poolEntries, poolID)
	}
}

// clear removes all entries from the cache
func (c *activeOrdersCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache.Purge()
	c.poolEntries = make(map[uint64]map[string]struct{})
}
