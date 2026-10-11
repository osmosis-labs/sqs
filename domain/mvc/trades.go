package mvc

import (
	"time"

	"github.com/osmosis-labs/sqs/domain"
)

// TradesRepository stores ingested trades in memory and maintains the rolling
// window aggregates incrementally as trades are stored and expire.
type TradesRepository interface {
	// StoreTrades appends one block's trades, which must be sorted ascending by key.
	// Trades at or before the last stored key are dropped, so re-storing a block is a no-op.
	// Also expires trades that left a window or the retention period as of now.
	StoreTrades(trades []domain.Trade, now time.Time)

	// Expire removes trades that left a window or the retention period as of now.
	Expire(now time.Time)

	// ListTrades returns up to limit trades matching the filter, newest first,
	// strictly older than the cursor if one is given. The returned cursor is nil
	// when there are no more trades.
	ListTrades(filter domain.TradesFilter, cursor *domain.TradeKey, limit int) ([]domain.Trade, *domain.TradeKey)

	// GetAggregates returns the aggregates for each requested window as of now.
	GetAggregates(filter domain.TradesFilter, windows []domain.TradeWindow, now time.Time) map[domain.TradeWindow]domain.TradeWindowAggregates

	// LastIngestedHeight returns the height of the last block the ingester
	// completed, or zero if none has been ingested since startup.
	LastIngestedHeight() int64

	// SetLastIngestedHeight records that the ingester completed the given height.
	SetLastIngestedHeight(height int64)
}
