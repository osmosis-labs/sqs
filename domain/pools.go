package domain

// code IDs for various implementations of the cosmwasm pool types
type CosmWasmCodeIDMaps struct {
	// code IDs for the transmuter pool type
	TransmuterCodeIDs map[uint64]struct{}
	// code IDs for the astroport pool type
	AstroportCodeIDs map[uint64]struct{}
}
