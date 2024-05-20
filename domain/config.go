package domain

import "fmt"

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
