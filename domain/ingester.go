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
