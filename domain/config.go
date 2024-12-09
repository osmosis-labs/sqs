package domain

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

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

	ChainTendermintRPCEndpoint string `mapstructure:"grpc-tendermint-rpc-endpoint"`
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

const envPrefix = "SQS"

var (
	DefaultConfig = Config{
		ServerAddress:              ":9092",
		LoggerFilename:             "sqs.log",
		LoggerIsProduction:         false,
		LoggerLevel:                "info",
		ChainTendermintRPCEndpoint: "http://localhost:26657",
		ChainGRPCGatewayEndpoint:   "localhost:9090",
		ChainID:                    "osmosis-1",
		ChainRegistryAssetsFileURL: "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/generated/frontend/assetlist.json",
		UpdateAssetsHeightInterval: 200,
		FlightRecord: &FlightRecordConfig{
			Enabled:          true,
			TraceThresholdMS: 1000,
			TraceFileName:    "/tmp/sqs-flight-record.trace",
		},
		Pools: &PoolsConfig{
			TransmuterCodeIDs: []uint64{
				148,
				254,
			},
			AlloyedTransmuterCodeIDs: []uint64{
				814,
				867,
				996,
			},
			OrderbookCodeIDs: []uint64{
				885,
			},
			GeneralCosmWasmCodeIDs: []uint64{
				503,
				572,
				773,
				641,
				842,
			},
		},
		Router: &RouterConfig{
			PreferredPoolIDs:                 []uint64{},
			MaxPoolsPerRoute:                 4,
			MaxRoutes:                        20,
			MaxSplitRoutes:                   3,
			MinPoolLiquidityCap:              0,
			RouteCacheEnabled:                true,
			CandidateRouteCacheExpirySeconds: 1200,
			RankedRouteCacheExpirySeconds:    45,
			DynamicMinLiquidityCapFiltersDesc: []DynamicMinLiquidityCapFilterEntry{
				{
					MinTokensCap: 1000000,
					FilterValue:  40000,
				},
				{
					MinTokensCap: 250000,
					FilterValue:  15000,
				},
				{
					MinTokensCap: 10000,
					FilterValue:  1000,
				},
				{
					MinTokensCap: 1000,
					FilterValue:  10,
				},
				{
					MinTokensCap: 1,
					FilterValue:  1,
				},
			},
		},
		Pricing: &PricingConfig{
			CacheExpiryMs:             2000,
			DefaultSource:             0,
			DefaultQuoteHumanDenom:    "usdc",
			MaxPoolsPerRoute:          4,
			MaxRoutes:                 3,
			MinPoolLiquidityCap:       1000,
			CoingeckoUrl:              "https://prices.osmosis.zone/api/v3/simple/price",
			CoingeckoQuoteCurrency:    "usd",
			WorkerMinPoolLiquidityCap: 1,
		},
		Passthrough: &passthroughdomain.PassthroughConfig{
			NumiaURL:                     "https://data.app.osmosis.zone",
			TimeseriesURL:                "https://data.stage.osmosis.zone",
			APRFetchIntervalMinutes:      5,
			PoolFeesFetchIntervalMinutes: 5,
		},
		GRPCIngester: &GRPCIngesterConfig{
			Enabled:                        true,
			MaxReceiveMsgSizeBytes:         16777216,
			ServerAddress:                  ":50051",
			ServerConnectionTimeoutSeconds: 10,
			Plugins: []Plugin{
				&OrderBookPluginConfig{
					Enabled: false,
					Name:    orderbookplugindomain.OrderbookFillbotPlugin,
				},
				&OrderBookPluginConfig{
					Enabled: false,
					Name:    orderbookplugindomain.OrderbookClaimbotPlugin,
				},
			},
		},
		OTEL: &OTELConfig{
			Enabled:     true,
			Environment: "sqs-dev",
		},
		CORS: &CORSConfig{
			AllowedHeaders: "Origin, Accept, Content-Type, X-Requested-With, X-Server-Time, Origin, Accept, Content-Type, X-Requested-With, X-Server-Time, Accept-Encoding, sentry-trace, baggage",
			AllowedMethods: "HEAD, GET, POST, HEAD, GET, POST, DELETE, OPTIONS, PATCH, PUT",
			AllowedOrigin:  "*",
		},
	}
)

// UnmarshalConfig handles the custom unmarshaling for the Config struct.
// Additionally, it sets up environment variable mappings using reflection.
// It also handles the Plugins field by decoding it using a custom decode hook.
// It uses Viper to handle environment variables and reflection to automatically generate environment variable mappings.
// CONTRACT: viper.ReadInConfig() is called before this function.
func UnmarshalConfig() (*Config, error) {
	config := DefaultConfig

	viper.SetEnvPrefix(envPrefix)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	// Set up environment variable mappings using reflection
	bindEnvRecursive(reflect.ValueOf(&config), "")

	// Use Viper's Unmarshal method to decode the configuration, except for the Plugins field
	if err := viper.Unmarshal(&config, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToSliceHookFunc(","),
			viperDecodeHookFunc(),
		)),
	); err != nil {
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

// Automatically generate environment variable mappings for all fields,
// including nested structs, without having to manually specify each binding.
// For example, if a mapstructure tag is "foo", the environment variable will be "SQS_FOO".
// If nested structs are present such as "foo.bar", the environment variable will be "SQS_FOO_BAR".
func bindEnvRecursive(v reflect.Value, prefix string) {
	t := v.Type()

	// Assume pointer to struct
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}

	// If not a struct after dereferencing, return
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// Get the mapstructure tag, if any
		tag := field.Tag.Get("mapstructure")
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}

		envName := prefix + tag

		// For nested structs, recurse
		if value.Kind() == reflect.Struct || (value.Kind() == reflect.Ptr && value.Elem().Kind() == reflect.Struct) {
			bindEnvRecursive(value, envName+".")
		} else {
			// Bind the environment variable
			if err := viper.BindEnv(envName); err != nil {
				panic(err)
			}
		}
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
	case orderbookplugindomain.OrderbookFillbotPlugin:
		return &OrderBookPluginConfig{}
	case orderbookplugindomain.OrderbookClaimbotPlugin:
		return &OrderBookPluginConfig{}
	// Add cases for other plugins as needed
	default:
		return nil
	}
}
