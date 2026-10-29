package coingeckopricing

import (
	"time"

	"github.com/osmosis-labs/sqs/domain"
)

// Tests run the bulk refresh on intervals of a few hundred milliseconds, below the production minimum.
func init() {
	minRefreshInterval = time.Millisecond
}

// StopRefresh stops the bulk refresh of a source created by New, so it does not outlive the test.
func StopRefresh(source domain.PricingSource) {
	source.(*coingeckoPricing).batcher.stop()
}
