package cache

import "time"

type RoutesOverwrite struct {
	cache *Cache
}

const noExpiration time.Duration = 0

// NewRoutesOverwrite creates a new routes overwrite container.
func NewRoutesOverwrite() *RoutesOverwrite {
	return &RoutesOverwrite{
		cache: New(),
	}
}

// NewNoOpRoutesOverwrite creates a new routes overwrite container that does nothing.
func NewNoOpRoutesOverwrite() *RoutesOverwrite {
	return &RoutesOverwrite{}
}

// CreateRoutesOverwrite creates a new routes overwrite container depending on the value of isRoutesOverwriteEnabled.
// If isRoutesOverwriteEnabled is true, it will return a new routes overwrite container.
// If isRoutesOverwriteEnabled is false, it will return a new no-op routes overwrite container.
func CreateRoutesOverwrite(isRoutesOverwriteEnabled bool) *RoutesOverwrite {
	if isRoutesOverwriteEnabled {
		return NewRoutesOverwrite()
	}
	return NewNoOpRoutesOverwrite()
}

// Set adds an item to the cache with a specified key and value.
// If the routes overwrite cache is not enabled, it will silently ignore the call.
func (r *RoutesOverwrite) Set(key string, value interface{}) {
	if r.cache == nil {
		return
	}

	r.cache.Set(key, value, noExpiration)
}

// Get retrieves the value associated with a key from the cache. Returns false if the key does not exist.
// If the routes overwrite cache is not enabled, it will silently ignore the call.
func (r *RoutesOverwrite) Get(key string) (interface{}, bool) {
	if r.cache == nil {
		return nil, false
	}

	return r.cache.Get(key)
}

// Delete removes an item from the cache.
// If the routes overwrite cache is not enabled, it will silently ignore the call.
func (r *RoutesOverwrite) Delete(key string) {
	if r.cache == nil {
		return
	}

	r.cache.Delete(key)
}
