package domain

type GRPCIngesterConfig struct {
	// Flag to enable the GRPC ingester server
	Enabled bool `mapstructure:"enabled"`

	// The maximum number of bytes to receive in a single GRPC message
	MaxReceiveMsgSizeBytes int `mapstructure:"max-receive-msg-size-bytes"`

	// The address of the GRPC ingester server
	ServerAddress string `mapstructure:"server-address"`

	// The number of seconds to wait for a connection to the server.
	ServerConnectionTimeoutSeconds int `mapstructure:"server-connection-timeout-seconds"`
}

// BlockPoolMetadata contains the metadata about unique pools
// and denoms modified in a block.
type BlockPoolMetadata struct {
	// DenomPoolLiquidityMap is a map of denoms to their liquidities across pools.
	// These contain all denoms and their liquidity data across all pools.
	DenomPoolLiquidityMap DenomPoolLiquidityMap
	// UpdatedDenoms are the denoms updated within a block.
	UpdatedDenoms map[string]struct{}
	// PoolIDs are the IDs of all pools updated within a block.
	PoolIDs map[uint64]struct{}
}
