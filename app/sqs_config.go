package main

import (
	"github.com/osmosis-labs/sqs/domain"
)

// Config defines the config for the sidecar query server.
type Config struct {
	// Storage defines the storage host and port.
	StorageHost string `mapstructure:"db-host"`
	StoragePort string `mapstructure:"db-port"`

	// Defines the web server configuration.
	ServerAddress             string `mapstructure:"server-address"`
	ServerTimeoutDurationSecs int    `mapstructure:"timeout-duration-secs"`

	// Defines the logger configuration.
	LoggerFilename     string `mapstructure:"logger-filename"`
	LoggerIsProduction bool   `mapstructure:"logger-is-production"`
	LoggerLevel        string `mapstructure:"logger-level"`

	ChainGRPCGatewayEndpoint string `mapstructure:"grpc-gateway-endpoint"`
	ChainID                  string `mapstructure:"chain-id"`

	// Router encapsulates the router config.
	Router *domain.RouterConfig `mapstructure:"router"`

	// Pools encapsulates the pools config.
	Pools *domain.PoolsConfig `mapstructure:"pools"`
}

// DefaultConfig defines the default config for the sidecar query server.
var DefaultConfig = Config{
	StorageHost: "localhost",
	StoragePort: "6379",

	ServerAddress:             ":9092",
	ServerTimeoutDurationSecs: 2,

	LoggerFilename:     "sqs.log",
	LoggerIsProduction: true,
	LoggerLevel:        "info",

	ChainGRPCGatewayEndpoint: "http://localhost:26657",
	ChainID:                  "osmosis-1",

	Router: &domain.RouterConfig{
		PreferredPoolIDs:        []uint64{},
		MaxPoolsPerRoute:        4,
		MaxRoutes:               5,
		MaxSplitRoutes:          3,
		MaxSplitIterations:      10,
		MinOSMOLiquidity:        10000, // 10_000 OSMO
		RouteCacheEnabled:       false,
		RouteCacheExpirySeconds: 600, // 10 minutes

		EnableOverwriteRoutesCache: false,
	},
	Pools: &domain.PoolsConfig{
		// This is what we have on mainnet as of Jan 2024.
		TransmuterCodeIDs: []uint64{148},
		AstroportCodeIDs:  []uint64{},
	},
}
