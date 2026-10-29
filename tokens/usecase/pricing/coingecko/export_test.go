package coingeckopricing

import "time"

// Tests run the bulk refresh on intervals of a few hundred milliseconds, below the production minimum.
func init() {
	minRefreshInterval = time.Millisecond
}
