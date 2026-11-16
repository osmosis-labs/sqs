package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
)

// Validates that a pricing cache key identifies one pair of denoms in one direction.
func TestFormatPricingCacheKey(t *testing.T) {
	// The price in one direction is the inverse of the price in the other, so the two
	// directions of a pair must not share an entry.
	require.NotEqual(t, domain.FormatPricingCacheKey("uosmo", "uion"), domain.FormatPricingCacheKey("uion", "uosmo"))

	// Different pairs must not join into the same key.
	require.NotEqual(t, domain.FormatPricingCacheKey("ab", "c"), domain.FormatPricingCacheKey("a", "bc"))
	require.NotEqual(t, domain.FormatPricingCacheKey("c", "ab"), domain.FormatPricingCacheKey("ca", "b"))
}
