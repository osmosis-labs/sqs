package domain

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/spf13/viper"
)

// Plugin defines the interface for the plugins.
type Plugin interface {
	IsEnabled() bool
	GetName() string
}

// Config defines the config for the sidecar query server.
type Config struct {
	// Defines the web server configuration.
	ServerAddress string `mapstructure:"server-address"`

	// Defines the logger configuration.
	LoggerFilename     string `mapstructure:"logger-filename"`
	LoggerIsProduction bool   `mapstructure:"logger-is-production"`
	LoggerLevel        string `mapstructure:"logger-level"`

	ChainTendermingRPCEndpoint string `mapstructure:"grpc-tendermint-rpc-endpoint"`
	ChainGRPCGatewayEndpoint   string `mapstructure:"grpc-gateway-endpoint"`
	ChainID                    string `mapstructure:"chain-id"`

	// Chain registry assets URL.
	ChainRegistryAssetsFileURL string `mapstructure:"chain-registry-assets-url"`

	// Defines the block interval at which the assets are updated.
	UpdateAssetsHeightInterval int `mapstructure:"update-assets-height-interval"`

	FlightRecord *FlightRecordConfig `mapstructure:"flight-record"`

	// Router encapsulates the router config.
	Router *RouterConfig `mapstructure:"router"`

	// Pools encapsulates the pools config.
	Pools *PoolsConfig `mapstructure:"pools"`

	Pricing *PricingConfig `mapstructure:"pricing"`

	// Passthrough encapsulates the passthrough module config.
	Passthrough *passthroughdomain.PassthroughConfig `mapstructure:"passthrough"`

	// GRPC ingester server configuration.
	GRPCIngester *GRPCIngesterConfig `mapstructure:"grpc-ingester"`

	// OpenTelemetry configuration.
	OTEL *OTELConfig `mapstructure:"otel"`

	// SideCarQueryServer CORS configuration.
	CORS *CORSConfig `mapstructure:"cors"`
}

// UnmarshalConfig handles the custom unmarshaling for the Config struct, particularly for Plugins.
func UnmarshalConfig() (*Config, error) {
	var config Config

	// Use Viper's Unmarshal method to decode the configuration, except for the Plugins field
	if err := viper.Unmarshal(&config, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToSliceHookFunc(","),
			viperDecodeHookFunc(),
		))); err != nil {
		return nil, err
	}

	return &config, nil
}

// viperDecodeHookFunc creates a custom decode hook to handle the Plugins field.
func viperDecodeHookFunc() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		// Default handling for non-Plugin fields
		if t != reflect.TypeOf([]Plugin(nil)) {
			return data, nil
		}

		// Cast the plugins data to a slice of interfaces
		pluginsDataSlice, ok := data.([]interface{})
		if !ok {
			return nil, errors.New("invalid plugins field")
		}

		plugins := make([]Plugin, 0)

		// Iterate over the plugins data and decode each plugin
		for _, pluginData := range pluginsDataSlice {
			// Cast the plugins map to a map[string]interface{}
			pluginsMap, ok := pluginData.(map[string]interface{})
			if !ok {
				return nil, errors.New("invalid plugins field")
			}

			name, ok := pluginsMap["name"].(string)
			if !ok {
				return nil, fmt.Errorf("plugin name missing or not a string")
			}

			plugin := PluginFactory(name)
			if plugin == nil {
				return nil, fmt.Errorf("unsupported plugin type: %s", name)
			}

			if err := mapstructure.Decode(pluginsMap, plugin); err != nil {
				return nil, err
			}

			plugins = append(plugins, plugin)
		}

		return plugins, nil
	}
}

// OrderBookPluginConfig encapsulates the order book plugin configuration.
type OrderBookPluginConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Name    string `mapstructure:"name"`
}

// GetName implements Plugin.
func (o *OrderBookPluginConfig) GetName() string {
	return o.Name
}

// IsEnabled implements Plugins.
func (o *OrderBookPluginConfig) IsEnabled() bool {
	return o.Enabled
}

var _ Plugin = &OrderBookPluginConfig{}

type EndpointOTELConfig struct {
	Quote float64 `mapstructure:"/router/quote"`
	Other float64 `mapstructure:"other"`
}

// OTELConfig represents OpenTelemetry configuration.
type OTELConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	Environment string `mapstructure:"environment"`
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

// PluginFactory creates a Plugin instance based on the provided name.
func PluginFactory(name string) Plugin {
	switch name {
	case orderbookplugindomain.OrderBookPluginName:
		return &OrderBookPluginConfig{}
	// Add cases for other plugins as needed
	default:
		return nil
	}
}
