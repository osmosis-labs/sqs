package domain

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// PricingSource defines the enumeration
// for possible pricing sources.
type PricingSource int

const (
	// ChainPricingSource defines the pricing source
	// by routing through on-chain pools.
	ChainPricingSource PricingSource = iota
	// CoinGeckoPricingSource defines the pricing source
	// that calls CoinGecko API.
	CoinGeckoPricingSource
)

// PricingStrategy defines an interface that must be fulfilled by the specific
// implementation of the pricing stratgy.
type PricingStrategy interface {
	// GetPrice returns the price given a bse and a quote denom or otherwise error, if any.
	GetPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error)
}
