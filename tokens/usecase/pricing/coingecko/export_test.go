package coingeckopricing

import (
	"time"

	"github.com/osmosis-labs/sqs/domain"
)

// Tests run the bulk refresh on intervals of a few hundred milliseconds, below the production minimum.
func init() {
	minRefreshInterval = time.Millisecond
}

// stop ends the bulk refresh. The batcher otherwise refreshes for as long as ids are tracked,
// which in production is the life of the process; tests stop it so that refreshes do not outlive
// the test servers they call.
func (b *priceBatcher) stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.quit:
	default:
		close(b.quit)
	}
}

// StopRefresh stops the bulk refresh of a source created by New, so it does not outlive the test.
func StopRefresh(source domain.PricingSource) {
	source.(*coingeckoPricing).batcher.stop()
}
