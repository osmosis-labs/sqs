package mvc

// ChainInfoUsecase is the interface that defines the methods for the chain info usecase
type ChainInfoUsecase interface {
	// GetLatestHeight returns the latest height stored
	// and returns an error if the height is stale.
	// That is, if the height has not been updated within a certain time frame.
	GetLatestHeight() (uint64, error)
	// StoreLatestHeight stores the latest height in the usecase
	StoreLatestHeight(height uint64)
	// ValidatePriceUpdates validates the price updates
	// Returns nil if the price updates are valid
	// Returns error otherwise.
	// Price updates can be invalid if:
	// - 50 heights have passed since the last update
	// - The initial price update has not been received
	ValidatePriceUpdates() error
	// ValidatePoolLiquidityUpdates validates the pool liquidity updates
	// Returns nil if the pool liquidity updates are valid
	// Returns error otherwise.
	// Pool liquidity updates can be invalid if:
	// - 50 heights have passed since the last update
	// - The initial pool liquidity update has not been received
	ValidatePoolLiquidityUpdates() error
}
