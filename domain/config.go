package domain

import "fmt"

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

// Validate validates the config. Returns an error if the config is invalid.
// Nil is returned if the config is valid.
func (c Config) Validate() error {
	// Validate the dynamic min liquidity cap filters.
	if err := validateDynamicMinLiquidityCapDesc(c.Router.DynamicMinLiquidityCapFiltersDesc); err != nil {
		return err
	}

	return nil
}

// validateDynamicMinLiquidityCapFiltersDesc validates the dynamic min liquidity cap filters.
// Returns an error if the filters are invalid. Nil is returned if the filters are valid.
// The filters must be in descending order both by min tokens capitalization and filter value.
func validateDynamicMinLiquidityCapDesc(values []DynamicMinLiquidityCapFilterEntry) error {
	if len(values) == 0 {
		return nil
	}

	previousMinTokensCap := values[0].MinTokensCap
	previousFilterValue := values[0].FilterValue
	for i := 0; i < len(values); i++ {
		if values[i].MinTokensCap > previousMinTokensCap {
			return fmt.Errorf("min_tokens_cap must be in descending order")
		}

		if values[i].FilterValue > previousFilterValue {
			return fmt.Errorf("filter_value must be in descending order")
		}

		previousMinTokensCap = values[i].MinTokensCap
		previousFilterValue = values[i].FilterValue
	}

	return nil
}
