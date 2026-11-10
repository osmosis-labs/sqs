package cache

import (
	"sync"
	"time"
)

// Cache is a concurrent cache structure.
//
// An expired item is removed when it is read, and by a sweep of all items that Set runs at
// most once per sweepInterval. Without the sweep, an item that is never read again would
// stay forever, and caches keyed by request input would grow for the life of the process.
type Cache struct {
	data  map[string]CacheItem
	mutex sync.RWMutex

	// nextSweep is the earliest time of the next sweep. Guarded by mutex.
	nextSweep time.Time
}

// CacheItem represents an item in the cache.
type CacheItem struct {
	Value      any
	Expiration time.Time
}

const NoExpirationTTL time.Duration = 0

// sweepInterval is the minimum time between two sweeps of expired items.
const sweepInterval = time.Minute

// New creates a new concurrent cache.
func New() *Cache {
	return &Cache{
		data:      make(map[string]CacheItem),
		nextSweep: time.Now().Add(sweepInterval),
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

// sweepExpiredLocked removes every expired item if a sweep is due.
// CONTRACT: mutex is held for writing.
func (c *Cache) sweepExpiredLocked(now time.Time) {
	if now.Before(c.nextSweep) {
		return
	}
	c.nextSweep = now.Add(sweepInterval)

	for key, item := range c.data {
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
