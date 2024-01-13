package domain

// CosmWasmPoolRouterConfig is the config for the CosmWasm pools in the router
type CosmWasmPoolRouterConfig struct {
	// code IDs for the transmuter pool type
	TransmuterCodeIDs map[uint64]struct{}
	// code IDs for the astroport pool type
	AstroportCodeIDs map[uint64]struct{}
	// node URI
	NodeURI string
}
