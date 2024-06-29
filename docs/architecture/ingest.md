# Ingest

This is a component that is responsible for ingesting and processing data.

It exposes a GRPC API for Osmosis node clients to send data to be ingested.

See [this ADR](https://www.notion.so/osmosiszone/ADR-006-Streaming-GRPC-SQS-Ingest-4e3b2ff7d23e43e2a1f3c43adc3c26bc) for details about the design.

The node [pushes](https://github.com/osmosis-labs/osmosis/blob/970db14d2ee14b4301bc6ebf6d620fa907923105/ingest/sqs/service/grpc_client.go#L42) only the pool data that is changed within a block.

As a result, it is possible to see the following sequence of events:
- Height X: All Osmosis pools are pushed
- Height X+1: Only the pools that have changed within that height are pushed

## Worker Pool Architecture

The ingest module is designed to be a worker pool that processes the data in parallel.
We configure the number of workers to be equal to 2. The choice is arbitrary but allows for an efficient catch up
post-initial cold start that takes roughly 30 seconds.

Given the target chain block time of 1.5 seconds, we are 15 blocks behind after cold start, and 2 workers can process them all
under a few seconds.

Additionally, this mechanism helps to control resources and avoid overloading the system at cold start with many pre-computation requests.

We keep a wait group in the `mvc.IngestUsecase` implementation to wait for the first block to finish processing before starting the next one.

Given the above architecture with block process jobs being queued up during start-up, the workers must differentiate updates by height.
That is, if a block process job for height X is being processed to compute a price for uosmo when uosmo already has a price for height X+1, the worker must discard the update for height X.

## Parsing Block Pool Metadata

Since we may push either all pools or only the ones updated within a block, we
maintain a data structure that helps to identify the pools and denoms that have been updated within a block
while retaining the context of all pools and denoms.

```go
// BlockPoolMetadata contains the metadata about unique pools
// and denoms modified in a block.
type BlockPoolMetadata struct {
	// DenomPoolLiquidityMap is a map of denoms to their liquidities across pools.
	// These contain all denoms and their liquidity data across all pools.
	DenomPoolLiquidityMap DenomPoolLiquidityMap
	// UpdatedDenoms are the denoms updated within a block.
	UpdatedDenoms map[string]struct{}
	// PoolIDs are the IDs of all pools updated within a block.
	PoolIDs map[uint64]struct{}
}
```

`UpdatedDenoms` and `PoolIDs` are used to identify the pools and denoms that have been updated within a block.

Initial `DenomPoolLiquidityMap` is constructed from the pool data that is pushed at the start of the block.

When the pool data is pushed, the `DenomPoolLiquidityMap` is updated with the new pool data.

We maintain [DenomPoolLiquidityMap](https://github.com/osmosis-labs/sqs/blob/83fbe8e25f332e259b97bc0a4873c21664d8f9f9/ingest/usecase/ingest_usecase.go#L59) in-memory of ingest usecase and [update it](https://github.com/osmosis-labs/sqs/blob/83fbe8e25f332e259b97bc0a4873c21664d8f9f9/ingest/usecase/ingest_usecase.go#L188) while processing each block.

## Workers

### Pricing

The pricing worker is responsible for pre-computing prices for all assets and the default quote denom (USDC).

It is triggerred by the ingester asyncronously when the GRPC push is received.

It looks at the unique denoms constructed from the pool data and computes the prices for each
asset and the default quote denom.

Once complete, it calls a hook to notify the subscribed listeners that the prices have been updated.

#### Pricing Listeners

- Healthcheck: The healthcheck listener is responsible for updating the healthcheck status based on the last time the prices were updated. If the prices are not updated within a certain time period, the healthcheck status will be updated to unhealthy.
- Pool Liquidity Pricing Worker: This worker is responsible for updating the pool liquidity capitalization
based on the prices that are computed by the pricing worker.

### Pool Liquidity Pricing

The pool liquidity pricing worker is responsible for computing liquidity capitalization (configured in USDC by default).

There are 2 kinds of liquidities capitalizations that we compute:
1. Token liquidity capitalization across all pools
  * Value of all pool liquidity of a token across all pools
2. Pool liqudity capitalization
  * Value of all pool balances in USDC

To compute the above, we need to know:
1. Prices for all tokens
2. Liquidity for all tokens in all pools

There are 2 possble cases:
1. All pools are being ingested and processed
   * This happens at cold start or if any error is returned in a previois block.
2. Only the pools that have changed within a height are being ingested
   * This assumes that all other pools have already been ingested in a previous height.


In the current system, When we push only a subset of pools, we do not need to read all ~1700 other pools. However, to commpute denom liquidity capitalization across all pools, we might need to know all pools that are associated with a particular token.

Therefore, we store the following data structure:

```go
// DenomPoolLiquidityData contains the pool liquidity data for a denom
// It has the total liquidity for the denom as well as all the
// pools with their individual contributions to the total.
type DenomPoolLiquidityData struct {
	// Total liquidity for this denom
	TotalLiquidity osmomath.Int
	// Mapping from pool ID to denom liquidity
	// in that pool
	Pools map[uint64]osmomath.Int
}

// DenomPoolLiquidityMap is a map of denoms to their pool liquidity data.
type DenomPoolLiquidityMap map[string]DenomPoolLiquidityData
```

This allows us to identify all pools that are associated with a particular token and read their liquidity
data without having to retrieve the entire pool model (or even read all pool liquidity values). Assume pool
1 is updated within a block and has denom ATOM. Then, we get `DenomPoolLiquidityData` for ATOM, subtract
the old liqudity contribution of that pool to `TotalLiquidity` and, finally add the new contributions to the total while also updating the entry in the `Pools` map.

In such a way, we only had to do 2 map accesses and 3 math operations while updating the liquidity for a token.

We [store this map in-memory of the ingester module](https://github.com/osmosis-labs/sqs/blob/81452a23b12fe9744e30ee04f5c13c790e404e51/ingest/usecase/ingest_usecase.go#L177), updating it while processing each block.

This map containing pool liquidity data for all pools is then [propagated to the pricing worker](https://github.com/osmosis-labs/sqs/blob/81452a23b12fe9744e30ee04f5c13c790e404e51/ingest/usecase/ingest_usecase.go#L95). Once prices are computed, they are pushed into the pool liquidity pricer together with the liquidity data for all pools.

By having the information about all pools updated within a block, their latest liquidity and prices of each token in the pool, we are able to recompute the liquidity capitalization for all updated pools and denom liquidities.

For example, assume that there is an ATOM/OSMO pool that is modified within a block. First, we recompute the default quote denom (USDC) denominated prices for ATOM and OSMO using the "pricing worker". Then, the "pool liquidity pricing worker" uses the updated prices from the pricing worker to recompute the capitalization (USDC-denominated value of total liquidity in the pool).

The denom liquidity capitalization and pool liquidity capitalizaion for each pool are computed concurrently by the
pool liquidity pricer worker after every block.

### Candidate Route Search Data

This worker is responsible for pre-computing the candidate route search pool data for each denom.

It relies on the pool liquidity capitalization data being available. As a result, it is implemented as its listener that is triggered by the pool liquidity pricing worker.

#### Pre-computing Method & Data Structures

Review the ingest data structures that we have defined earliet in this file:
- `DenomPoolLiquidityData`
- `DenomPoolLiquidityMap`
- `BlockPoolMetadata`


Now, we introduce:

```go
type CandidateRoutePoolData map[string][]sqsdomain.PoolI
```

It will be stored as a `sync.Map` in the router handler.

To achive its construction, we rely on 

- `BlockPoolMetadata` and all pools with their liquidity capitalizations computed.

Process:
1. Using the `BlockPoolMetadata.UpdatedDenoms`, we know which denoms have been updated
and need their candidate route pool data recomputed.
2. For each denom:
   a) Grab its pool IDs from `BlockPoolMetadata.DenomPoolLiquidityMap`.
   b) Use [mvc.PoolsHandler.GetPools([]uint64)](https://github.com/osmosis-labs/sqs/blob/83fbe8e25f332e259b97bc0a4873c21664d8f9f9/pools/usecase/pools_usecase.go#L249) to retrieve the pools.
      - Note: there is a minor risk of contention with client requests.
   c) Sort the pools according to the pool filtering algorithm described above in this document.
   d) Store in router handler with `mvc.RouterHandler.StoreCandidateRoutePoolData(string, []sqsdomain.PoolI)`.
