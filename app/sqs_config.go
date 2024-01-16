package main

import (
	"github.com/osmosis-labs/sqs/domain"
)

// DefaultConfig defines the default config for the sidecar query server.
var DefaultConfig = domain.Config{
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
		TransmuterCodeIDs:      []uint64{148},
		GeneralCosmWasmCodeIDs: []uint64{},
	},
}
