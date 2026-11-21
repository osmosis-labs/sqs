package cache

import "github.com/prometheus/client_golang/prometheus"

// Values of the reason label of EvictionsCounter.
const (
	// EvictionReasonExpired is an entry that had already expired but was not swept yet.
	EvictionReasonExpired = "expired"
	// EvictionReasonUnexpired is an entry that had not expired: the cache's entry cap cost it a
	// live entry, which a cold cache never does.
	EvictionReasonUnexpired = "unexpired"
)

const (
	cacheLabel  = "cache"
	reasonLabel = "reason"
)

// sqs_cache_capacity_evictions_total
//
// counter of entries evicted because a cache was at its entry cap when a new key was set.
//
// Has the following labels:
// * cache - the name the cache was created with
// * reason - "expired" or "unexpired"
var EvictionsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "sqs_cache_capacity_evictions_total",
		Help: "Total number of entries evicted because a cache was at its entry cap",
	},
	[]string{cacheLabel, reasonLabel},
)

func init() {
	prometheus.MustRegister(EvictionsCounter)
}
