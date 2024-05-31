package domain

// Config defines the config for the sidecar query server.
type Config struct {
	// Defines the web server configuration.
	ServerAddress string `mapstructure:"server-address"`

	// Defines the logger configuration.
	LoggerFilename     string `mapstructure:"logger-filename"`
	LoggerIsProduction bool   `mapstructure:"logger-is-production"`
	LoggerLevel        string `mapstructure:"logger-level"`

	ChainGRPCGatewayEndpoint string `mapstructure:"grpc-gateway-endpoint"`
	ChainID                  string `mapstructure:"chain-id"`

	// Chain registry assets URL.
	ChainRegistryAssetsFileURL string `mapstructure:"chain-registry-assets-url"`

	FlightRecord *FlightRecordConfig `mapstructure:"flight-record"`

	// Router encapsulates the router config.
	Router *RouterConfig `mapstructure:"router"`

	// Pools encapsulates the pools config.
	Pools *PoolsConfig `mapstructure:"pools"`

	Pricing *PricingConfig `mapstructure:"pricing"`

	// GRPC ingester server configuration.
	GRPCIngester *GRPCIngesterConfig `mapstructure:"grpc-ingester"`

	// OpenTelemetry configuration.
	OTEL *OTELConfig `mapstructure:"otel"`

	// SideCarQueryServer CORS configuration.
	CORS *CORSConfig `mapstructure:"cors"`
}

type EndpointOTELConfig struct {
	Quote float64 `mapstructure:"/router/quote"`
	Other float64 `mapstructure:"other"`
}

// OTELConfig represents OpenTelemetry configuration.
type OTELConfig struct {
	// The DSN to use.
	DSN string `mapstructure:"dsn"`
	// The sample rate for event submission in the range [0.0, 1.0].
	// By default, all events are sent.
	SampleRate float64 `mapstructure:"sample-rate"`
	// Enable performance tracing.
	EnableTracing bool `mapstructure:"enable-tracing"`
	// The sample rate for profiling traces in the range [0.0, 1.0].
	// This is relative to TracesSampleRate - it is a ratio of profiled traces out of all sampled traces.
	ProfilesSampleRate float64 `mapstructure:"profiles-sample-rate"`
	// The environment to be sent with events.
	Environment      string             `mapstructure:"environment"`
	CustomSampleRate EndpointOTELConfig `mapstructure:"custom-sample-rate"`
}

// CORSConfig represents HTTP CORS headers configuration.
type CORSConfig struct {
	// Specifies Access-Control-Allow-Headers header value.
	AllowedHeaders string `mapstructure:"allowed-headers"`
	// Specifies Access-Control-Allow-Methods header value.
	AllowedMethods string `mapstructure:"allowed-methods"`
	// Specifies Access-Control-Allow-Origin header value.
	AllowedOrigin string `mapstructure:"allowed-origin"`
}

// FlightRecordConfig encapsulates the flight recording configuration.
type FlightRecordConfig struct {
	// Enabled defines if the flight recording is enabled.
	Enabled bool `mapstructure:"enabled"`
	// TraceThresholdMS defines the trace threshold in milliseconds.
	TraceThresholdMS int `mapstructure:"trace-threshold-ms"`
	// TraceFileName defines the trace file name to output to.
	TraceFileName string `mapstructure:"trace-file-name"`
}
