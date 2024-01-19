# Sidecar Query Server

This is a sidecar query server that is used for performing query tasks outside of the main chain.
The high-level architecture is that the chain reads data at the end of the block, parses it
and then writes into a Redis instance.

The sidecar query server then reads the parsed data from Redis and serves it to the client
via HTTP endpoints.

The use case for this is performing certain data and computationally intensive tasks outside of
the chain node or the clients. For example, routing falls under this category because it requires
all pool data for performing the complex routing algorithm.

## Integrator Guide

Follow [this link](https://hackmd.io/@3DOBr1TJQ3mQAFDEO0BXgg/S1bsqPAr6) to find a guide on how to 
integrate with the sidecar query server.

## Custom CosmWasm Pools

The sidecar query server supports custom CosmWasm pools.
There are two options of integrating them into the Osmosis router:
1. Implement a pool type similar to [transmuter](https://github.com/osmosis-labs/sqs/blob/e95c66e3ee6a22d57118c74a384253f016a9bb85/router/usecase/pools/routable_transmuter_pool.go#L19)
   * This assumes that the pool quote and spot price logic is trivial enough for implementing
   it directly in SQS without having to interact with the chain.
2. Utilize a [generalized CosmWasm pool type](https://github.com/osmosis-labs/sqs/blob/437086c683f4f90d915f7e042617552c68410796/router/usecase/pools/routable_cw_pool.go#L24)
   * This assumes that the pool quote and spot price logic is complex enough for requiring
   interaction with the chain.
   * For quotes and spot prices, SQS service would make network API queries to the chain.
   * This is the simplest approach but it is less performant than the first option.
   * Due to performance reasons, the routes containing these pools are not utilized in
   more performant split quotes. Only direct quotes are supported.

To enable support for either option, a [config.json](https://github.com/osmosis-labs/sqs/blob/437086c683f4f90d915f7e042617552c68410796/config.json#L22-L25)
must be updated accordingly. For option 1, add a new field under `pools` and make a PR propagating
this config to be able to create a new custom pool type similar to transmuter. For option 2, simply
add your code id to `general-cosmwasm-code-ids` in this repository. Tag `@p0mvn` in the PR and
follow up that the config is deployed to the sidecar query server service in production.

## Supported Endpoints

### Pools Resource

1. GET `/pools/all`

Description: returns all pools in the chain state instrumented with balances, pool type and
spread factor.

Parameters: none

Response example:
```bash
curl "https://sqs.osmosis.zone/pools/all" | jq .
[
  {
    "chain_model": {
      "address": "osmo1mw0ac6rwlp5r8wapwk3zs6g29h8fcscxqakdzw9emkne6c8wjp9q0t3v8t",
      "id": 1,
      "pool_params": {
        "swap_fee": "0.002000000000000000",
        "exit_fee": "0.000000000000000000"
      },
      "future_pool_governor": "24h",
      "total_weight": "1073741824000000.000000000000000000",
      "total_shares": {
        "denom": "gamm/pool/1",
        "amount": "68705408290810473783205087"
      },
      "pool_assets": [
        {
          "token": {
            "denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
            "amount": "1099238954791"
          },
          "weight": "536870912000000"
        },
        {
          "token": {
            "denom": "uosmo",
            "amount": "6560268370850"
          },
          "weight": "536870912000000"
        }
      ]
    },
    "balances": [
      {
        "denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
        "amount": "1099238954791"
      },
      {
        "denom": "ibc/9989AD6CCA39D1131523DB0617B50F6442081162294B4795E26746292467B525",
        "amount": "1000000000"
      },
      {
        "denom": "ibc/B9E0A1A524E98BB407D3CED8720EFEFD186002F90C1B1B7964811DD0CCC12228",
        "amount": "999800"
      },
      {
        "denom": "uosmo",
        "amount": "6560268370850"
      }
    ],
    "type": 0,
    "spread_factor": "0.002000000000000000"
  },
  ...
]
```

2. GET `/pools/:id`

Description: returns the pool with the given id instrumented with sqs-specific data for routing.

Arguments: id of a pool

Response example:
```bash
curl "https://sqs.osmosis.zone/pools/1" | jq .
{
"chain_model": {
    "address": "osmo1mw0ac6rwlp5r8wapwk3zs6g29h8fcscxqakdzw9emkne6c8wjp9q0t3v8t",
    "id": 1,
    "pool_params": {
    "swap_fee": "0.002000000000000000",
    "exit_fee": "0.000000000000000000"
    },
    "future_pool_governor": "24h",
    "total_weight": "1073741824000000.000000000000000000",
    "total_shares": {
    "denom": "gamm/pool/1",
    "amount": "68705408290810473783205087"
    },
    "pool_assets": [
    {
        "token": {
        "denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
        "amount": "1099238954791"
        },
        "weight": "536870912000000"
    },
    {
        "token": {
        "denom": "uosmo",
        "amount": "6560268370850"
        },
        "weight": "536870912000000"
    }
    ]
},
"balances": [
    {
    "denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
    "amount": "1099238954791"
    },
    {
    "denom": "ibc/9989AD6CCA39D1131523DB0617B50F6442081162294B4795E26746292467B525",
    "amount": "1000000000"
    },
    {
    "denom": "ibc/B9E0A1A524E98BB407D3CED8720EFEFD186002F90C1B1B7964811DD0CCC12228",
    "amount": "999800"
    },
    {
    "denom": "uosmo",
    "amount": "6560268370850"
    }
],
"type": 0,
"spread_factor": "0.002000000000000000"
}
```

3. GET `/pools?IDs=<poolIDs>`

Description: same as `/pools/all` or `/pools/:id` with the exception that it
allows batch fetching of specific pools by the given parameter pool IDs.

Parameter: `IDs` - the list of pool IDs to batch fetch.

```
curl "http://localhost:9092/pools?IDs=1,2" | jq .
[
  {
    "chain_model": {
      "address": "osmo1mw0ac6rwlp5r8wapwk3zs6g29h8fcscxqakdzw9emkne6c8wjp9q0t3v8t",
      "id": 1,
      "pool_params": {
        "swap_fee": "0.002000000000000000",
        "exit_fee": "0.000000000000000000"
      },
      "future_pool_governor": "24h",
      "total_weight": "1073741824000000.000000000000000000",
      "total_shares": {
        "denom": "gamm/pool/1",
        "amount": "68705408290810473783205087"
      },
      "pool_assets": [
        {
          "token": {
            "denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
            "amount": "1099147835604"
          },
          "weight": "536870912000000"
        },
        {
          "token": {
            "denom": "uosmo",
            "amount": "6560821009725"
          },
          "weight": "536870912000000"
        }
      ]
    },
    "balances": [
      {
        "denom": "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2",
        "amount": "1099147835604"
      },
      {
        "denom": "ibc/9989AD6CCA39D1131523DB0617B50F6442081162294B4795E26746292467B525",
        "amount": "1000000000"
      },
      {
        "denom": "ibc/B9E0A1A524E98BB407D3CED8720EFEFD186002F90C1B1B7964811DD0CCC12228",
        "amount": "999800"
      },
      {
        "denom": "uosmo",
        "amount": "6560821009725"
      }
    ],
    "type": 0,
    "spread_factor": "0.002000000000000000"
  },
  ...
]
```

4. GET `/pools/ticks/:id`

Description: returns the tick data for the given concentrated pool ID.
Returns non-200 if the pool is not concentrated or does not exist.

Argument: id of a pool

Response example:
```bash
curl "https://sqs.osmosis.zone/pools/ticks/1221" | jq .
# Response ommitted for brevity
```

### Router Resource


1. GET `/router/quote?tokenIn=<tokenIn>&tokenOutDenom=<tokenOutDenom>`

Description: returns the best quote it can compute for the given tokenIn and tokenOutDenom

Parameters:
- `tokenIn` the string representation of the sdk.Coin for the token in
- `tokenOutDenom` the string representing the denom of the token out

Response example:

```bash
curl "https://sqs.osmosis.zone/router/quote?tokenIn=1000000uosmo&tokenOutDenom=uion" | jq .
{
  "amount_in": {
    "denom": "uosmo",
    "amount": "1000000"
  },
  "amount_out": "1803",
  "route": [
    {
      "pools": [
        {
          "id": 2,
          "type": 0,
          "balances": [],
          "spread_factor": "0.005000000000000000",
          "token_out_denom": "uion",
          "taker_fee": "0.001000000000000000"
        }
      ],
      "out_amount": "1803",
      "in_amount": "1000000"
    }
  ],
  "effective_fee": "0.006000000000000000"
}
```

2. GET `/router/single-quote?tokenIn=<tokenIn>&tokenOutDenom=<tokenOutDenom>`

Description: returns the best quote it can compute w/o performing route splits,
performing single direct route estimates only.

Parameters:
- `tokenIn` the string representation of the sdk.Coin for the token in
- `tokenOutDenom` the string representing the denom of the token out

Response example:
```bash
curl "https://sqs.osmosis.zone/router/single-quote?tokenIn=1000000uosmo&tokenOutDenom=uion" | jq .
{
  "amount_in": {
    "denom": "uosmo",
    "amount": "1000000"
  },
  "amount_out": "1803",
  "route": [
    {
      "pools": [
        {
          "id": 2,
          "type": 0,
          "balances": [],
          "spread_factor": "0.005000000000000000",
          "token_out_denom": "uion",
          "taker_fee": "0.001000000000000000"
        }
      ],
      "out_amount": "1803",
      "in_amount": "1000000"
    }
  ],
  "effective_fee": "0.006000000000000000"
}
```

3. GET `/router/routes?tokenIn=<tokenIn>&tokenOutDenom=<tokenOutDenom>`

Description: returns all routes that can be used for routing from tokenIn to tokenOutDenom

Parameters:
- `tokenIn` the string representation of the denom of the token in
- `tokenOutDenom` the string representing the denom of the token out


Response example:
```bash
curl "https://sqs.osmosis.zone/router/routes?tokenIn=uosmo&tokenOutDenom=uion" | jq .
{
  "Routes": [
    {
      "Pools": [
        {
          "ID": 1100,
          "TokenOutDenom": "uion"
        }
      ]
    },
    {
      "Pools": [
        {
          "ID": 2,
          "TokenOutDenom": "uion"
        }
      ]
    },
    {
      "Pools": [
        {
          "ID": 1013,
          "TokenOutDenom": "uion"
        }
      ]
    },
    {
      "Pools": [
        {
          "ID": 1092,
          "TokenOutDenom": "ibc/E6931F78057F7CC5DA0FD6CEF82FF39373A6E0452BF1FD76910B93292CF356C1"
        },
        {
          "ID": 476,
          "TokenOutDenom": "uion"
        }
      ]
    },
    {
      "Pools": [
        {
          "ID": 1108,
          "TokenOutDenom": "ibc/9712DBB13B9631EDFA9BF61B55F1B2D290B2ADB67E3A4EB3A875F3B6081B3B84"
        },
        {
          "ID": 26,
          "TokenOutDenom": "uion"
        }
      ]
    }
  ],
  "UniquePoolIDs": {
    "1013": {},
    "1092": {},
    "1100": {},
    "1108": {},
    "2": {},
    "26": {},
    "476": {}
  }
}
```

4. GET `/router/custom-quote?tokenIn=<tokenIn>&tokenOutDenom=<tokenOutDenom>&poolIDs=<poolIDs>`

Description: returns the quote over route with the given poolIDs. If such route does not exist, returns error.
This endpoint uses the router route search. As a result, it is affected by the minimum liquidity parameter
in the config. If your desired pool does not appead in the router, thy decreasing the minimum liquidity
parameter. Alternatively, you can use the `/router/custom-direct-quote` endpoint.

Parameters:
- `tokenIn` the string representation of the sdk.Coin for the token in
- `tokenOutDenom` the string representing the denom of the token out
- `poolIDs` comma-separated list of pool IDs

Response example:
```bash
curl "https://sqs.osmosis.zone/router/custom-quote?tokenIn=1000000uosmo&tokenOutDenom=uion&poolIDs=2" | jq .
{
  "amount_in": {
    "denom": "uosmo",
    "amount": "1000000"
  },
  "amount_out": "1803",
  "route": [
    {
      "pools": [
        {
          "id": 2,
          "type": 0,
          "balances": [],
          "spread_factor": "0.005000000000000000",
          "token_out_denom": "uion",
          "taker_fee": "0.001000000000000000"
        }
      ],
      "out_amount": "1803",
      "in_amount": "1000000"
    }
  ],
  "effective_fee": "0.006000000000000000"
}
```

5. GET `/router/custom-direct-quote?tokenIn=<tokenIn>&tokenOutDenom=<tokenOutDenom>&poolIDs=<poolIDs>`

Description: returns the quote over route with the given poolIDs. If such route does not exist, returns error.
This endpoint does not use the router route search. As a result, it is not affected by the minimum liquidity parameter. As long as the pool exists on-chain, it will return a quote.

Parameters:
- `tokenIn` the string representation of the sdk.Coin for the token in
- `tokenOutDenom` the string representing the denom of the token out
- `poolID` comma-separated list of pool IDs

Response example:
```bash
curl "https://sqs.osmosis.zone/router/custom-direct-quote?tokenIn=1000000uosmo&tokenOutDenom=uion&poolID=2" | jq .
{
  "amount_in": {
    "denom": "uosmo",
    "amount": "1000000"
  },
  "amount_out": "1803",
  "route": [
    {
      "pools": [
        {
          "id": 2,
          "type": 0,
          "balances": [],
          "spread_factor": "0.005000000000000000",
          "token_out_denom": "uion",
          "taker_fee": "0.001000000000000000"
        }
      ],
      "out_amount": "1803",
      "in_amount": "1000000"
    }
  ],
  "effective_fee": "0.006000000000000000"
}
```

6. GET `/router/cached-routes?tokenIn=uosmo&tokenOutDenom=uion`

Description: returns cached routes for the given tokenIn and tokenOutDenomn if cache
is enabled. If not, returns error. Contrary to `/router/routes...` endpoint, does
not attempt to compute routes if cache is not enabled.

Parameters: none

Parameters:
- `tokenIn` the string representation of the denom of the token in
- `tokenOutDenom` the string representing the denom of the token out


Response example:
```bash
curl "https://sqs.osmosis.zone/cached-routes?tokenIn=uosmo&tokenOutDenom=uion" | jq .
{
  "Routes": [
    {
      "Pools": [
        {
          "ID": 1100,
          "TokenOutDenom": "uion"
        }
      ]
    },
    {
      "Pools": [
        {
          "ID": 2,
          "TokenOutDenom": "uion"
        }
      ]
    },
    {
      "Pools": [
        {
          "ID": 1013,
          "TokenOutDenom": "uion"
        }
      ]
    },
    {
      "Pools": [
        {
          "ID": 1092,
          "TokenOutDenom": "ibc/E6931F78057F7CC5DA0FD6CEF82FF39373A6E0452BF1FD76910B93292CF356C1"
        },
        {
          "ID": 476,
          "TokenOutDenom": "uion"
        }
      ]
    },
    {
      "Pools": [
        {
          "ID": 1108,
          "TokenOutDenom": "ibc/9712DBB13B9631EDFA9BF61B55F1B2D290B2ADB67E3A4EB3A875F3B6081B3B84"
        },
        {
          "ID": 26,
          "TokenOutDenom": "uion"
        }
      ]
    }
  ],
  "UniquePoolIDs": {
    "1013": {},
    "1092": {},
    "1100": {},
    "1108": {},
    "2": {},
    "26": {},
    "476": {}
  }
}
```

7. POST `/router/store-state`

Description: stores the current state of the router in a JSON file locally. Used for debugging purposes.
This endpoint should be disabled in production.

Parameters: none

8. GET `/router/spot-price-pool/:id`

Parameters:
- `quoteAsset` the quote asset denom
- `baseAsset` the base asset denom

```bash
curl "https://sqs.osmosis.zone/router/spot-price-pool/1212?quoteAsset=ibc/D189335C6E4A68B513C10AB227BF1C1D38C746766278BA3EEB4FB14124F1D858&baseAsset=ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
"1.000000000000000000000000000000000000"
```

### System Resource

1. GET `/healthcheck`

Description: returns 200 if the server is healthy.
Validates the following conditions:
- Redis is reachable
- Node is reachable
- Node is not syncing
- The latest height in Redis is within threshold of the latest height in the node
- The latest height in Redis was updated within a configurable number of seconds

2. GET `/metrics`

Description: returns the prometheus metrics for the server

3. GET `/version`

Description: returns the version of the server

4. GET `/config`

Description: returns the configuration of the server, including the router.

## Development Setup

### Mainnet

To setup a development environment against mainnet, sync the node in the default
home directory and then run the following commands:

```bash
# Starts a detached redis container, to stop: 'make redis-stop'
make redis-start

# Rebuild the binary and start the node with sqs enabled in-process
make sqs-start
```

### Localosmosis

It is also possible to run the sidecar query server against a localosmosis node.

```bash
# Starts localosmosis with all services enabled and a few pools pre-created
# make localnset-start for empty state
# See localosmosis docs for more details
make localnet-start-with-state
```

## Data

### Pools

For every chain pool, its pool model is written to Redis.

Additionally, we instrument each pool model with bank balances and OSMO-denominated TVL.

Some pool models do not contain balances by default. As a result, clients have to requery balance
for each pool model directly from chain. Having the balances in Redis allows us to avoid this and serve
pools with balances directly.

The routing algorithm requires the knowledge of TVL for prioritizing pools. As a result, each pool model
is instrumented with OSMO-denominated TVL.

### Router

For routing, we must know about the taker fee for every denom pair. As a result, in the router
repository, we stote the taker fee keyed by the denom pair.

These taker fees are then read from Redis to initialize the router.

### Token Precision

The chain is agnostic to token precision. As a result, to compute OSMO-denominated TVL,
we query [chain registry file](https://github.com/osmosis-labs/assetlists/blob/main/osmosis-1/osmosis-1.assetlist.json)
parse the precision exponent and use it scaling the spot price to the right value.

The following are the tokens that are either malformed or are missing from the chain registry file:
```md
ibc/CD942F878C80FBE9DEAB8F8E57F592C7252D06335F193635AF002ACBD69139CC
ibc/FE2CD1E6828EC0FAB8AF39BAC45BC25B965BA67CCBC50C13A14BD610B0D1E2C4
ibc/4F3B0EC2FE2D370D10C3671A1B7B06D2A964C721470C305CBB846ED60E6CAA20
ibc/CD20AC50CE57F1CF2EA680D7D47733DA9213641D2D116C5806A880F508609A7A
ibc/52E12CF5CA2BB903D84F5298B4BFD725D66CAB95E09AA4FC75B2904CA5485FEB
ibc/49C2B2C444B7C5F0066657A4DBF19D676E0D185FF721CFD3E14FA253BCB9BC04
ibc/7ABF696369EFB3387DF22B6A24204459FE5EFD010220E8E5618DC49DB877047B
ibc/E27CD305D33F150369AB526AEB6646A76EC3FFB1A6CA58A663B5DE657A89D55D
factory/osmo130w50f7ta00dxkzpxemuxw7vnj6ks5mhe0fr8v/oDOGE
ibc/5BBB6F9C8ECA31508EE5B68F2E27B57532E1595C57D0AE5C8D64E1FBCB756247
ibc/00BC6883C29D45EAA021A55CFDD5884CA8EFF9D39F698A9FEF79E13819FF94F8
ibc/BCDB35B7390806F35E716D275E1E017999F8281A81B6F128F087EF34D1DFA761
ibc/020F5162B7BC40656FC5432622647091F00D53E82EE8D21757B43D3282F25424
ibc/D3A1900B2B520E45608B5671ADA461E1109628E89B4289099557C6D3996F7DAA
ibc/1271ACDB6421652A2230DECCAA365312A32770579C2B22D2B60A89FE39106611
ibc/DEA3B0BB0006C69E75D2247E8DC57878758790556487067F67748FDC237CE2AE
ibc/72D0C53912C461FC9251E3135459746380E9030C0BFDA13D45D3BAC47AE2910E
ibc/0E30775281643124D79B8670ACD3F478AC5FAB2B1CA1E32903D0775D8A8BB064
ibc/4E2A6E691D0EB60A25AE582F29D19B20671F723DF6978258F4043DA5692120AE
ibc/F2F19568D75125D7B88303ADC021653267443679780D6A0FD3E1EC318E0C51FD
factory/osmo19pw5d0jset8jlhawvkscj2gsfuyd5v524tfgek/TURKEY
```

Any pool containing these tokens would have the TVL error error set to
non-empty string, leading to the pool being deprioritized from the router.

### Algorithm

In this section, we describe the general router algorithm.

1. Retrieve pools from storage.
2. Filter out low liquidity pools.
3. Rank pools by several heuristics such as:
 - liquidity
 - pool type (priority: transmuter, concentrated, stableswap, balancer)
 - presence of error in TVL computation.
4. Compute candidate routes
   * For the given token in and token out denom, find all possible routes
   between them using the pool ranking discussed above as well as by limiting
   the algorithm per configuration.
   * The configurations are:
      * Max Hops: The maximum number of hops allowed in a route.
      * Max Routes: The maximum number of routes to consider.
   * The algorithm that is currently used is breadth first search.
5. Compute the best quote when swapping amount in in-full directly over each route.
6. Sort routes by best quote.
7. Keep "Max Splittable Routes" and attempt to determine an optimal quote split across them
   * If the split quote is more optimal, return that. Otherwise, return the best single direct quote.

### Caching

We perform caching of routes to avoid having to recompute them on every request.
The routes are cached in a Redis instance.

There is a configuration parameter that enables the route cache to be updated every X blocks.
However, that is an experimental feature. See the configuration section for details.

The router also caches the routes when it computes it for the first time for a given token in and token out denom.
As of now, the cache is cleared at the end of very block. We should investigate only clearing pool data but persisting
the routes for longer while allowing for manual updates and invalidation.

### Configuration

The router has several configuration parameters that are set via `app.toml`.

See the recommended enabled configuration below:
```toml
###############################################################################
###              Osmosis Sidecar Query Server Configuration                 ###
###############################################################################

[osmosis-sqs]

# SQS service is disabled by default.
is-enabled = "true"

# The hostname and address of the sidecar query server storage.
db-host = "localhost"
db-port = "6379"

# Defines the web server configuration.
server-address = ":9092"
timeout-duration-secs = "2"

# Defines the logger configuration.
logger-filename = "sqs.log"
logger-is-production = "true"
logger-level = "info"

# Defines the gRPC gateway endpoint of the chain.
grpc-gateway-endpoint = "http://localhost:26657"

# The list of preferred poold IDs in the router.
# These pools will be prioritized in the candidate route selection, ignoring all other
# heuristics such as TVL.
preferred-pool-ids = []

# The maximum number of pools to be included in a single route.
max-pools-per-route = "4"

# The maximum number of routes to be returned in candidate route search.
max-routes = "20"

# The maximum number of routes to be split across. Must be smaller than or
# equal to max-routes.
max-split-routes = "3"

# The maximum number of iterations to split a route across.
max-split-iterations = "10"

# The minimum liquidity of a pool to be included in a route.
min-osmo-liquidity = "10000"

# The height interval at which the candidate routes are recomputed and updated in
# Redis
route-update-height-interval = "0"

# Whether to enable candidate route caching in Redis.
route-cache-enabled = "true"
```
