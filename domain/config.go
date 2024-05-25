package domain

// Config defines the config for the sidecar query server.
type Config struct {
	// Storage defines the storage host and port.
	StorageHost string `mapstructure:"db-host"`
	StoragePort string `mapstructure:"db-port"`

	// Defines the web server configuration.
	ServerAddress string `mapstructure:"server-address"`

	// Defines the logger configuration.
	LoggerFilename     string `mapstructure:"logger-filename"`
	LoggerIsProduction bool   `mapstructure:"logger-is-production"`
	LoggerLevel        string `mapstructure:"logger-level"`

	ChainGRPCGatewayEndpoint string `mapstructure:"grpc-gateway-endpoint"`
	ChainID                  string `mapstructure:"chain-id"`

	// Chain registry assets firl URL.
	ChainRegistryAssetsFileURL string `mapstructure:"chain-registry-assets-url"`

	// Router encapsulates the router config.
	Router *RouterConfig `mapstructure:"router"`

	// Pools encapsulates the pools config.
	Pools *PoolsConfig `mapstructure:"pools"`

	Pricing *PricingConfig `mapstructure:"pricing"`

	GRPCIngester *GRPCIngesterConfig `mapstructure:"grpc-ingester"`

	OTEL *OTELConfig `mapstructure:"otel"`

	CORS *CORSConfig `mapstructure:"cors"`
}

type EndpointOTELConfig struct {
	Quote float64 `mapstructure:"/router/quote"`
	Other float64 `mapstructure:"other"`
}

type OTELConfig struct {
	DSN                string             `mapstructure:"dsn"`
	SampleRate         float64            `mapstructure:"sample-rate"`
	EnableTracing      bool               `mapstructure:"enable-tracing"`
	TracesSampleRate   float64            `mapstructure:"traces-sample-rate"`
	ProfilesSampleRate float64            `mapstructure:"profiles-sample-rate"`
	Environment        string             `mapstructure:"environment"`
	CustomSampleRate   EndpointOTELConfig `mapstructure:"custom-sample-rate"`
}

type CORSConfig struct {
	AllowedHeaders string `mapstructure:"allowed-headers"`
	AllowedMethods string `mapstructure:"allowed-methods"`
	AllowedOrigin  string `mapstructure:"allowed-origin"`
}
