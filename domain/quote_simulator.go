package domain

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// QuoteSimulator simulates a quote and returns the gas adjusted amount and the fee coin.
type QuoteSimulator interface {
	// SimulateQuote simulates a quote and returns the gas adjusted amount and the fee coin.
	// CONTRACT:
	// - Only direct (non-split) quotes are supported.
	// Retursn error if:
	// - Simulator address does not have enough funds to pay for the quote.
	SimulateQuote(ctx context.Context, quote Quote, slippageToleranceMultiplier osmomath.Dec, simulatorAddress string) TxFeeInfo
}
