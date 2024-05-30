package domain

// CosmWasmPoolRouterConfig is the config for the CosmWasm pools in the router
type CosmWasmPoolRouterConfig struct {
	// code IDs for the transmuter pool type
	TransmuterCodeIDs map[uint64]struct{}
	// code IDs for the generalized cosmwasm pool type
	GeneralCosmWasmCodeIDs map[uint64]struct{}

	// isAlloyedTransmuterEnabled is a flag to enable the alloyed transmuter pool type
	IsAlloyedTransmuterEnabled bool

	// node URI
	NodeURI string
}
