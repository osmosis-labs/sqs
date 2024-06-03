# Routing

This document outlines the details about routing.

## Pool Filtering - Min Liquidity Capitalization

Osmosis chain consists of many pools where some of them are low liqudity.
Having these pools be included in the router system imposes performance overhead
without clear benefit.

Additionally, if 2 concurrent user swaps go over the same pool with low liquidity, one
of them is likely to exhaust liquidity, either causing a mistestimate to the other user
(that frequently goes unnoticed in-practice) or simply fails the swap.

To work around this constraint, we introduce a min liquidity capitalization filter.

This filter requires the pools to meet a certain threshold to be eligible for router inclusion.

However, applying the same filter universally introduces another constraint - swaps between low-liqudiity 
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

#### Ingestion

Note that this assumes the existence of mapping from denoms to their respective liquidities
across all pools. We enable this by iterating over all pools during the time of ingest,
computing token liquidities, storing them in the in-memory cache.

#### Configuration

We configure the mappings from min liqudity capitalization to filters via the following config:
`router.dynamic-min-liquidity-cap-filters-desc`.

It represents a slice of sorted in descending order by-liquidity entries. We omit further details for brevity.

If filters are unspecified, we fallback to the default and universal `router.min-pool-liquidity-cap`.

**Imporant:** it is worth noting that both the total liquidity capitalization values across all pools
and the configuration parameters are normalized. That is, they assume having the appropriate scaling factors
applied.

#### Example

Consider the following configuration of pool liquidity capitalization to filter value translations:
```
1_000_000 -> 100_000
50_000 -> 10_000
```

This implies that tokens from total liquidity across all pools of over or equal to $1M require the min
liquidity capitalization filter of $100K. Similarly, tokens with liquidities over $100K and below $1M
are required to 
