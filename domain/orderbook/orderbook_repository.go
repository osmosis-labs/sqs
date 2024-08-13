package orderbookdomain

type OrderBookRepository interface {
	// StoreTicks stores the orderbook ticks for a given orderbook pool id.
	StoreTicks(poolID uint64, ticksMap map[int64]OrderbookTick)

	// GetTicks returns the orderbook ticks for a given orderbook pool id.
	GetAllTicks(poolID uint64) (map[int64]OrderbookTick, bool)

	// GetTicks returns specific orderbook ticks for a given orderbook pool id.
	// Errors if at least one tick is not found.
	GetTicks(poolID uint64, tickIDs []int64) (map[int64]OrderbookTick, error)

	// GetTickByID returns a specific orderbook tick for a given orderbook pool id.
	// Returns false if the tick is not found.
	GetTickByID(poolID uint64, tickID int64) (OrderbookTick, bool)
}
