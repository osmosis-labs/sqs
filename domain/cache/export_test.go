package cache

import "time"

const NoExpiration time.Duration = 0

// SweepBatchSize is the maximum number of items that one Set examines for expiry.
const SweepBatchSize = sweepBatchSize

// EvictSampleSize is the maximum number of items sampled when evicting a single entry at capacity.
const EvictSampleSize = evictSampleSize

// EvictOne evicts one entry from c as a Set of a new key at capacity would, at time now.
func EvictOne(c *Cache, now time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.evictOneLocked(now)
}
