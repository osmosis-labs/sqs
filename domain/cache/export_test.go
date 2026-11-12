package cache

import "time"

const NoExpiration time.Duration = 0

// SweepBatchSize is the maximum number of items that one Set examines for expiry.
const SweepBatchSize = sweepBatchSize
