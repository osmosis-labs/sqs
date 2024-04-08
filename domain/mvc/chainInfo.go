package mvc

// ChainInfoUsecase is the interface that defines the methods for the chain info usecase
type ChainInfoUsecase interface {
	// GetLatestHeight returns the latest height stored
	// and returns an error if the height is stale.
	// That is, if the height has not been updated within a certain time frame.
	GetLatestHeight() (uint64, error)
	// StoreLatestHeight stores the latest height in the usecase
	StoreLatestHeight(height uint64)
}
