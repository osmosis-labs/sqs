# Routing

This document outlines the details about routing.

## Algorithm

In this section, we describe the general router algorithm.

It is split into 2 parts: a) pre-computed at ingest time b) computed at runtime of client request

### Pre-computed

1. Retrieve pools from storage.
2. Filter out no liquidity pools.
3. For each denom, create linking to pools it routes to.
   * Rank pools by several heuristics such as:
    -   liquidity
    -   pool type (priority: transmuter, alloyed, concentrated, stableswap, balancer)
    -   presence of error in TVL computation.

### Client Runtime

1. Compute candidate routes
    - For the given token in and token out denom, find all possible routes
      between them using the pool ranking discussed above as well as by limiting
      the algorithm per configuration.
    - The configurations are:
        - Max Hops: The maximum number of hops allowed in a route.
        - Max Routes: The maximum number of routes to consider.
    - The algorithm that is currently used is breadth first search using pre-computed associations between tokens & pools.
2. Compute the best quote when swapping amount in in-full directly over each route. By in-full, we mean as if all amount in
is consumed by a single route as opposed to performing partial split routing over many routes.
3. Sort routes by best quote.
4. Keep "Max Splittable Routes" and attempt to determine an optimal quote split across them
    - If the split quote is more optimal, return that. Otherwise, return the best single direct quote.

## Route Cache

We perform caching of routes to avoid having to recompute them on every request.

The caches are configured via TTL in the config. There is also a boolean config to enable it.

We have two types of route caches:

1. **Candidate route**

This cache aims to contain `router.max-routes` number of unranked routes between token in and token out denom.

2. **Ranked route**

This cache aims to contain top `router.max-split-routes` that are ranked by token out amount across all top candidate routes. Only the top `router.max-split-routes` are written to cache.

For a given token in and out denom, this cache is written with the granularity of order of magnitude of token in because
the top routes can drastically vary as the token in amount changes due to varying pool liquidities.

## Pool Filtering - Min Liquidity Capitalization

Osmosis chain consists of many pools where some of them are low liquidity.
Having these pools be included in the router system imposes performance overhead
without clear benefit.

Additionally, if 2 concurrent user swaps go over the same pool with low liquidity, one
of them is likely to exhaust liquidity, either causing a misestimate to the other user
(that frequently goes unnoticed in-practice) or simply fails the swap.

To work around this constraint, we introduce a min liquidity capitalization filter.

This filter requires the pools to meet a certain threshold to be eligible for router inclusion.

However, applying the same filter universally introduces another constraint - swaps between low-liquidity 
tokens might fail to construct routes due to not meeting a high threshold.

Given a large number of tokens with varying liquidity, finding the perfect min liquidity capitalization
parameter to satisfy all constraints is untenable.

As a result, we introduce the concept of "Dynamic Min Liquidity Capitalization".

### Dynamic Min Liquidity Capitalization

This feature enables computing the min liquidity capitalization parameter dynamically based
on the token in and out denom.

The pseudocode for this is the following:
```
# Get the minimum token liquidity across all pools between token in and token out.
min_token_liq = min(total_liq[tokenInDenom], total_liq[tokenOutDenom])

# Use the minimum token liquidity to get the appropriate min liquidity capitalization filter.
dynamic_min_liq_cap = map_token_liq_to_liq_cap(min_token_liq)
```

1. Get the minimum token liquidity across all pools between token in and token out.
2. Use the minimum token liquidity to get the appropriate min liquidity capitalization filter.

### Orderbooks

Orderbooks are not affected by the minimum liquidity capitalization filter. For each token pair, we maintain a canonical orderbook with the highest liquidity. We keep this orderbook because:

- Limit orders can be placed on any tick, regardless of liquidity. Therefore, we avoid prematurely filtering out orderbooks and exclude them from caches.

Consequently, orderbooks are always prioritized and never filtered in pool liquidity filtering. We always include the canonical orderbook route in the route caches, even if better routes are available. For example, if the maximum number of routes is three, we keep those three routes and also include the canonical orderbook route in the cache.

#### Ingestion

Note that this assumes the existence of mapping from denoms to their respective liquidities
across all pools. We enable this by iterating over all pools during the time of ingest,
computing token liquidities, storing them in the in-memory cache.

#### Configuration

We configure the mappings from min liquidity capitalization to filters via the following config:
`router.dynamic-min-liquidity-cap-filters-desc`.

It represents a slice of sorted in descending order by-liquidity entries. We omit further details for brevity.

If filters are unspecified, we fallback to the default and universal `router.min-pool-liquidity-cap`.

**Important:** it is worth noting that both the total liquidity capitalization values across all pools
and the configuration parameters are normalized. That is, they assume having the appropriate scaling factors
applied.

#### Example

Consider the following configuration of pool liquidity capitalization to filter value translations:
```
1_000_000 -> 100_000
50_000 -> 10_000
```

And a default parameter for min liquidity capitalization of $1K.

This implies that tokens with total liquidity across all pools of over or equal to $1M require the min
liquidity capitalization filter of $100K. Similarly, tokens with liquidities over $50K and below $1M
are required to route over pools with min liquidity capitalization of $10K.

Assume we have the following liquidity capitalizations across all pools for the tokens:
```
- ATOM -> $2M
- JUNO -> $300K
- BONK -> $1K
```

Consider the following examples:

1. Swap ATOM for JUNO
```
# min(2_000_000, 300_000) = 300_000
min_token_liq = min(total_liq[ATOM], total_liq[JUNO])

# Translate $300K to $10K since $300K > $50K per configuration.
dynamic_min_liq_cap = map_token_liq_to_liq_cap(min_token_liq)
```

2. Swap ATOM for BONK
```
# min(2_000_000, 1_000) = 1_000
min_token_liq = min(total_liq[ATOM], total_liq[BONK])

# Translate $1K to $1K (default) since $1K under $50K (lowest threshold value per configuration).
dynamic_min_liq_cap = map_token_liq_to_liq_cap(min_token_liq)
```

The reason for choosing the minimum of the total pool liquidities between token in and token out is
so that we can still find routes between low liquidity tokens that likely have pools of even smaller liquidity.

#### API

The dynamic min liquidity capitalization feature is enabled by default with the fallback to the universal
default quote.

For eligible routing endpoints:
- `/router/quote`
- `/router/custom-direct-quote`

In some cases, clients may want to disable the fallback, preferring to error rather than have a potential
for bad route stemming from the low universal default.

The query parameter of `disableMinLiquidityFallback` disables the fallback, returning an error instead.
