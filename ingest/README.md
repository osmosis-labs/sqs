# Ingest

This is a component that is responsible for ingesting and processing data.

It exposes a GRPC API for Osmosis node clients to send data to be ingested.

See [this ADR](https://www.notion.so/osmosiszone/ADR-006-Streaming-GRPC-SQS-Ingest-4e3b2ff7d23e43e2a1f3c43adc3c26bc) for details about the design.

The node pushes only the pool data that is changed within a block.

As a result, it is possible to see the following sequence of events:
- Height X: All Osmosis pools are pushed
- Height X+1: Only the pools that have changed within that height are pushed

## Workers

### Pricing

The pricing worker is responsible for pre-computing prices for all assets and the default
quote denom (USDC).

It is triggerred by the ingester asyncrnously when the GRPC push is received.

It looks at the unique denoms constructed from the pool data and computes the prices for each
asset and the default quote denom.

Once complete, it calls a hook to notify the subscribed listeners that the prices have been updated.

#### Pricing Listeners

- Healthcheck: The healthcheck listener is responsible for updating the healthcheck status
  based on the last time the prices were updated. If the prices are not updated within a certain
  time period, the healthcheck status will be updated to unhealthy.
An example of a listener is the healtcheck component. Once it is notified of the price update, it
- Pool Liquidity Pricing Worker: This worker is responsible for updating the pool liquidity capitalization
based on the prices that are computed by the pricing worker.

### Pool Liquidity Pricing

The pool liquidity pricing worker is responsible for updating the pool liquidity capitalization.

We need to know:
1. Prices for all tokens
2. Liquidity for all tokens in all pools

Again, 2 cases:

1. All pools are pushed
2. Only the pools that have changed within that height are pushed

Problem:
- In the current system, When we push only a subset of pools, we do not need to read all ~1700 other pools.

However, with the pool liquidity pricing in place, we might need to know all pools that are associated with
a particular token.

Therefore, we store the following data structure:

```go
map[denom]struct{
    TotalLiquidity: sdk.Int
    map[poolId]struct{
        Liquidity: sdk.Int
    }
}
```

This would alow us to identify all pools that are associated with a particular token and read them only
rather than reading all pools.