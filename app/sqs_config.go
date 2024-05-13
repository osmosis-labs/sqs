package main

import (
	"github.com/osmosis-labs/sqs/domain"
)

// DefaultConfig defines the default config for the sidecar query server.
var DefaultConfig = domain.Config{
	StorageHost: "localhost",
	StoragePort: "6379",

	ServerAddress: ":9092",

	LoggerFilename:     "sqs.log",
	LoggerIsProduction: true,
	LoggerLevel:        "info",

	ChainGRPCGatewayEndpoint:   "http://localhost:26657",
	ChainID:                    "osmosis-1",
	ChainRegistryAssetsFileURL: "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/osmosis-1.assetlist.json",

	Router: &domain.RouterConfig{
		PreferredPoolIDs:                 []uint64{},
		MaxPoolsPerRoute:                 4,
		MaxRoutes:                        5,
		MaxSplitRoutes:                   3,
		MaxSplitIterations:               10,
		MinOSMOLiquidity:                 100, // 100 OSMO
		RouteCacheEnabled:                false,
		CandidateRouteCacheExpirySeconds: 600, // 10 minutes
		RankedRouteCacheExpirySeconds:    300, // 5 minutes

		EnableOverwriteRoutesCache: false,
	},
	Pools: &domain.PoolsConfig{
		// This is what we have on mainnet as of Jan 2024.
		TransmuterCodeIDs:      []uint64{148},
		GeneralCosmWasmCodeIDs: []uint64{},
	},

	Pricing: &domain.PricingConfig{
		DefaultSource:          domain.ChainPricingSourceType,
		CacheExpiryMs:          2000, // 2 seconds.
		DefaultQuoteHumanDenom: "usdc",

		MaxPoolsPerRoute:       4,
		MaxRoutes:              5,
		MinOSMOLiquidity:       50,
		CoingeckoUrl:           "https://prices.osmosis.zone/api/v3/simple/price",
		CoingeckoQuoteCurrency: "usd",
	},
}
