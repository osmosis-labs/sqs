package domain

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// QuoteSimulator simulates a quote and returns the gas adjusted amount and the fee coin.
type QuoteSimulator interface {
	// SimulateQuote simulates a quote and returns the gas adjusted amount and the fee coin.
	// CONTRACT:
	// - Only direct (non-split) quotes are supported.
	// Retursn error if:
	// - Simulator address does not have enough funds to pay for the quote.
	SimulateQuote(ctx context.Context, quote Quote, slippageToleranceMultiplier osmomath.Dec, simulatorAddress string) (uint64, sdk.Coin, error)
}

type QuotePriceInfo struct {
	AdjustedGasUsed uint64   `json:"adjusted_gas_used"`
	FeeCoin         sdk.Coin `json:"fee_coin"`
}
