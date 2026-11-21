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
//
// To protect memory against bursts of unique unexpired keys within a TTL window, the cache also
// enforces a hard entry cap (maxEntries). When a new key arrives at capacity, a small random
// sample is inspected and the first expired item, or otherwise the soonest-expiring item, is
// evicted. Get remains a pure read-lock lookup.
type Cache struct {
	data       map[string]CacheItem
	maxEntries int
	mutex      sync.RWMutex
}

// CacheItem represents an item in the cache.
type CacheItem struct {
	Value      any
	Expiration time.Time
}

const NoExpirationTTL time.Duration = 0

// DefaultMaxEntries bounds the number of items a cache created with New can hold.
const DefaultMaxEntries = 50_000

// sweepBatchSize is the maximum number of items that one Set examines for expiry. It bounds the
// time Set holds the write lock independently of the cache size.
const sweepBatchSize = 32

// evictSampleSize is the maximum number of items sampled when evicting a single entry at capacity.
const evictSampleSize = 16

// New creates a new concurrent cache bounded to DefaultMaxEntries.
func New() *Cache {
	return NewWithMaxEntries(DefaultMaxEntries)
}

// NewWithMaxEntries creates a new concurrent cache bounded to maxEntries items.
// A non-positive maxEntries falls back to DefaultMaxEntries.
func NewWithMaxEntries(maxEntries int) *Cache {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	return &Cache{
		data:       make(map[string]CacheItem),
		maxEntries: maxEntries,
	}
}

// Set adds an item to the cache with a specified key, value, and expiration time.
func (c *Cache) Set(key string, value any, expiration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	c.sweepExpiredLocked(now)

	if c.maxEntries > 0 && len(c.data) >= c.maxEntries {
		if _, exists := c.data[key]; !exists {
			c.evictOneLocked(now)
		}
	}

	expirationTime := time.Time{}
	if expiration != NoExpirationTTL {
		expirationTime = now.Add(expiration)
	}
	c.data[key] = CacheItem{
		Value:      value,
		Expiration: expirationTime,
	}
}

// evictOneLocked removes one entry from the cache to make room for a new key.
// It inspects up to evictSampleSize randomly iterated entries, preferring an expired entry
// first, then the entry with the earliest expiration, and falling back to the first sampled
// entry when all sampled entries have no expiration.
// CONTRACT: mutex is held for writing and len(c.data) > 0.
func (c *Cache) evictOneLocked(now time.Time) {
	var (
		victimKey   string
		victimExp   time.Time
		hasVictim   bool
		sampled     int
		fallbackKey string
		hasFallback bool
	)

	for key, item := range c.data {
		if sampled == evictSampleSize {
			break
		}
		sampled++

		if !hasFallback {
			fallbackKey = key
			hasFallback = true
		}

		if !item.Expiration.IsZero() {
			if now.After(item.Expiration) {
				delete(c.data, key)
				return
			}
			if !hasVictim || item.Expiration.Before(victimExp) {
				victimKey = key
				victimExp = item.Expiration
				hasVictim = true
			}
		}
	}

	if hasVictim {
		delete(c.data, victimKey)
	} else if hasFallback {
		delete(c.data, fallbackKey)
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
