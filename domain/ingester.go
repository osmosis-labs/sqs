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
	// DenomMap is a map of denoms.
	// These are constructed from the pool IDs updated within a block.
	DenomMap DenomMap
	// PoolIDs are the IDs of all pools updated within a block.
	PoolIDs map[uint64]struct{}
}
