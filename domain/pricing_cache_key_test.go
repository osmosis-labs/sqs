package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
)

// Validates that a pricing cache key identifies one pair of denoms, in either order.
func TestFormatPricingCacheKey(t *testing.T) {
	// A pair shares one entry in both directions.
	require.Equal(t, domain.FormatPricingCacheKey("uosmo", "uion"), domain.FormatPricingCacheKey("uion", "uosmo"))

	// Different pairs must not join into the same key.
	require.NotEqual(t, domain.FormatPricingCacheKey("ab", "c"), domain.FormatPricingCacheKey("a", "bc"))
	require.NotEqual(t, domain.FormatPricingCacheKey("c", "ab"), domain.FormatPricingCacheKey("bc", "a"))
}
