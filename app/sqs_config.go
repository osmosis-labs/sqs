package main

import (
	"github.com/osmosis-labs/sqs/domain"
)

// DefaultConfig defines the default config for the sidecar query server.
var DefaultConfig = domain.Config{
	ServerAddress: ":9092",

	LoggerFilename:     "sqs.log",
	LoggerIsProduction: true,
	LoggerLevel:        "info",

	ChainTendermingRPCEndpoint: "http://localhost:26657",
	ChainGRPCGatewayEndpoint:   "http://localhost:9090",
	ChainID:                    "osmosis-1",
	ChainRegistryAssetsFileURL: "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/generated/frontend/assetlist.json",
	UpdateAssetsHeightInterval: 200,

	Router: &domain.RouterConfig{
		PreferredPoolIDs:                 []uint64{},
		MaxPoolsPerRoute:                 4,
		MaxRoutes:                        5,
		MaxSplitRoutes:                   3,
		MinPoolLiquidityCap:              100, // The denomination assummed is set by Pricing.DefaultHumanDenom
		RouteCacheEnabled:                false,
		CandidateRouteCacheExpirySeconds: 600, // 10 minutes
		RankedRouteCacheExpirySeconds:    300, // 5 minutes
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
		MinPoolLiquidityCap:    50,
		CoingeckoUrl:           "https://prices.osmosis.zone/api/v3/simple/price",
		CoingeckoQuoteCurrency: "usd",
	},
}
