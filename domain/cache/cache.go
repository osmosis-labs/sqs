package cache

import (
	"sync"
	"time"
)

// Cache is a concurrent cache structure.
//
// An expired item is removed when it is read, and by an incremental sweep that every Set runs
// over at most sweepBatchSize items. Without the sweep, an item that is never read again would
// stay forever, and caches keyed by request input would grow for the life of the process.
//
// The sweep is bounded rather than periodic and complete: a full pass holds the write lock for
// time linear in the cache size and stalls every concurrent Get on the quote path. Map
// iteration starts at a random position, so successive batches sample the whole cache, and each
// Set examines more items than it adds, which keeps the share of expired items bounded.
type Cache struct {
	data  map[string]CacheItem
	mutex sync.RWMutex
}

// CacheItem represents an item in the cache.
type CacheItem struct {
	Value      any
	Expiration time.Time
}

const NoExpirationTTL time.Duration = 0

// sweepBatchSize is the maximum number of items that one Set examines for expiry. It bounds the
// time Set holds the write lock independently of the cache size.
const sweepBatchSize = 32

// New creates a new concurrent cache.
func New() *Cache {
	return &Cache{
		data: make(map[string]CacheItem),
	}
}

// Set adds an item to the cache with a specified key, value, and expiration time.
func (c *Cache) Set(key string, value any, expiration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	c.sweepExpiredLocked(now)

	expirationTime := time.Time{}
	if expiration != NoExpirationTTL {
		expirationTime = now.Add(expiration)
	}
	c.data[key] = CacheItem{
		Value:      value,
		Expiration: expirationTime,
	}
}

// sweepExpiredLocked examines at most sweepBatchSize items, starting at a random position, and
// removes the expired ones. A cache of at most sweepBatchSize items is swept completely.
// CONTRACT: mutex is held for writing.
func (c *Cache) sweepExpiredLocked(now time.Time) {
	examined := 0
	for key, item := range c.data {
		if examined == sweepBatchSize {
			return
		}
		examined++

		if !item.Expiration.IsZero() && now.After(item.Expiration) {
			delete(c.data, key)
		}
	}
}

// Get retrieves the value associated with a key from the cache.
func (c *Cache) Get(key string) (any, bool) {
	c.mutex.RLock()

	item, exists := c.data[key]
	if !exists {
		c.mutex.RUnlock()
		return nil, false
	}

	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		// Unlock before locking again
		c.mutex.RUnlock()

		// Acquire write mutex. The entry is re-read under it: a concurrent Set may have
		// replaced the expired item with a fresh one between the two locks, and that one
		// must be neither deleted nor missed.
		c.mutex.Lock()
		defer c.mutex.Unlock()

		item, exists = c.data[key]
		if !exists {
			return nil, false
		}
		if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
			delete(c.data, key)
			return nil, false
		}
		return item.Value, true
	}

	c.mutex.RUnlock()

	return item.Value, true
}

// Delete removes an item from the cache.
func (c *Cache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.data, key)
}

// Len returns the number of entries in the cache
func (c *Cache) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return len(c.data)
}
