# Implementation Checkpoint: Orderbook Capacity-Aware Force-Add

## Summary

This PR implements **Option 2** from the OSMO-53 fix proposal: "Only Force-Add if Orderbook Has Capacity". The canonical orderbook route is now only force-added to cached routes when it has sufficient liquidity for the requested swap amount.

## Problem Solved

Previously, the canonical orderbook was **always** force-added back to cached routes after ranking (even when filtered out due to insufficient liquidity). This caused issues for large swaps like 500K USDC → BTC, where the low-liquidity orderbook pool (Pool 1930, ~$42K liquidity) would be reconsidered on every request.

## Changes

### `router/usecase/router_usecase.go`

- Added `orderbookHasSufficientCapacity()` helper function that checks if an orderbook pool can handle the requested swap amount
- Modified the force-add logic to only re-add canonical orderbook if:
  - Pool data can be fetched successfully
  - OrderbookData is available
  - `tokenIn.Amount <= capacity` (using same mirrored data as quote calculation)
- Added 4 debug log points prefixed with `OSMO-53:` for troubleshooting:
  - `OSMO-53: candidate routes found`
  - `OSMO-53: routes after ranking`
  - `OSMO-53: orderbook capacity check`
  - `OSMO-53: canonical orderbook force-add decision`

### `router/usecase/export_test.go`

- Exported `OrderbookHasSufficientCapacity()` for testing
- Exported `SetPoolsUsecase()` for test setup

### `router/usecase/router_usecase_test.go`

- Added `TestOrderbookHasSufficientCapacity()` with 9 test cases:
  - Sufficient capacity for BID direction
  - Insufficient capacity for BID direction (OSMO-53 scenario)
  - Exact capacity match
  - Sufficient capacity for ASK direction
  - Insufficient capacity for ASK direction
  - Nil CosmWasmPoolModel handling
  - Nil OrderbookData handling
  - Pool fetch error handling
  - Zero capacity handling

## Developer Concern Addressed

> "There's a part of the SQS code that mirrors the contract to avoid expensive queries, we'd need to ensure we don't cause invalid quotes"

**Resolution**: The capacity check uses the **same** `OrderbookData` (specifically `BidAmountToExhaustAskLiquidity` / `AskAmountToExhaustBidLiquidity`) that is already used in `CalculateTokenOutByTokenIn()` for quote calculation. No new data source is introduced, so no new staleness concerns exist.

## How It Works With OSMO-53

```
500K USDC request
    │
    ▼
Route ranking: Test full 500K
    - Orderbook: Error (insufficient capacity)
    - CL pools: Error (too much for single route)
    │
    ▼
PROBE FALLBACK (optimized_routes.go fix):
    Test 50K probe amount
    - Orderbook: Error (still over 42K capacity)  
    - CL pools: Success
    │
    ▼
Split algorithm runs with CL pools
    │
    ▼
CACHE WRITE (this fix):
    - Orderbook was in candidates but filtered
    - Capacity check: 42K < 500K → SKIP force-add
    - Cache only contains working CL routes
    │
    ▼
Valid quote returned
```

## Testing

```bash
# Run capacity check unit tests
go test -v -run TestOrderbookHasSufficientCapacity ./router/usecase/...

# Run all router tests
go test -v ./router/usecase/...

# Enable debug logs (set in config or env)
SQS_LOGGER_LEVEL=debug
```

## Next Steps

- [ ] Code review
- [ ] Run full test suite
- [ ] Deploy to staging for OSMO-53 scenario testing
- [ ] Verify debug logs show expected behavior
