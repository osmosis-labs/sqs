# OSMO-53: Large Swap Routing Fix

## Problem
500K USDC→BTC swap returns **94% slippage** (~0.4 BTC instead of ~4.4 BTC) because router selects low-liquidity orderbook pool (Pool 1930, ~$42K liquidity) due to its 0% spread factor.

---

## Behavior Comparison

### 🔴 Previous (Production)
```
500K USDC request
    ↓
Route ranking: Test full 500K on each route
    ↓
Split algorithm: Orderbook error SILENTLY IGNORED
    - Code: `result, _ := route.CalculateTokenOutByTokenIn(...)`
    ↓
❌ Bad route passed to frontend → 94% slippage
```

### 🟡 Current (Staging)
```
440K USDC request
    ↓
Route ranking: Test full 440K on each route
    - All routes ERROR at this amount
    ↓
❌ "No routes provided" error → User can't swap
```
**Fix applied**: Errors now surfaced in `dynamic_splits.go`  
**New issue**: All routes filtered out before split algorithm runs

### 🟢 Proposed
```
440K USDC request
    ↓
Route ranking: Test full 440K → All routes error
    ↓
FALLBACK: Test probe amount (10% = 44K)
    - Orderbook: Error (still over capacity)
    - CL pools: ✅ Success
    ↓
Split algorithm: Allocates across working CL pools
    ↓
✅ Valid quote returned
```

---

## Changes Required

| File | Status | Change |
|------|--------|--------|
| `dynamic_splits.go` | ✅ Done | Handle route errors (return 0 instead of ignoring) |
| `optimized_routes.go` | 🔧 Needed | Add probe fallback when all routes fail at full amount |

---

## Why Fallback (Not First-Line Probe)?

| Approach | Normal Swaps | Large Swaps |
|----------|--------------|-------------|
| **Fallback** | Fast (1 test) | 2 tests |
| **Always probe** | Slower (probe + split always) | Same |

Most traffic is normal-sized → fallback keeps them fast, only pays extra cost for large swaps.

---

## Edge Cases Handled

| Scenario | Result |
|----------|--------|
| Probe still too big for all routes | "No routes" error (correct - truly no capacity) |
| Probe works for orderbook too | Split algorithm errors on larger increments → excludes it |

**Defense in depth**: Probe identifies viable routes → Split algorithm handles capacity limits dynamically.

---

## Test Cases

- [ ] 400K USDC→BTC: Should work (baseline)
- [ ] 440K USDC→BTC: Should work after fix
- [ ] 500K USDC→BTC: Should work with reasonable slippage (not 94%)
- [ ] Small amounts (<$10K): Should remain fast (no probe triggered)


