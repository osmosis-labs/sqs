# Pools

## Canonical Orderbooks

The pools use case provides functionality for identifying the canonical orderbook pools among all orderbook-type pools.

The canonical orderbook is defined as the orderbook with the highest liquidity for a given token pair.

During the ingestion process, as the system stores pools, it processes the orderbook pools to determine the canonical orderbook for each token pair.

The system then exposes an API that allows querying the canonical orderbook for a specific token pair as well as retrieving all canonical orderbooks for all token pairs.
