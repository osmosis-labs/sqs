package cache

import "time"

const NoExpiration time.Duration = 0

// SetNextSweep sets the earliest time of the next sweep of expired items.
func (c *Cache) SetNextSweep(t time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.nextSweep = t
}
