package domain

// CosmWasmPoolRouterConfig is the config for the CosmWasm pools in the router
type CosmWasmPoolRouterConfig struct {
	// code IDs for the transmuter pool type
	TransmuterCodeIDs map[uint64]struct{}
	// code IDs for the generalized cosmwasm pool type
	GeneralCosmWasmCodeIDs map[uint64]struct{}
	// node URI
	NodeURI string
}

// BlockPoolMetadata contains the metadata about unique pools
// and denoms modified in a block.
type BlockPoolMetadata struct {
	UpdatedDenoms     map[string]struct{}
	DenomLiquidityMap map[string]PoolDenomMetaData
	PoolIDs           map[uint64]struct{}
}
