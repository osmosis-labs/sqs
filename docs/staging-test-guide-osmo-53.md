# Staging Test Guide: OSMO-53 Fix Verification

## Overview

This guide covers testing the fix for **OSMO-53: SQS Router causes 94% slippage for large USDC to BTC swaps**.

### Fix Summary
The dynamic programming (DP) split algorithm now properly handles route errors (e.g., orderbook pools with insufficient liquidity) by treating them as zero-output routes, preventing them from being incorrectly selected for large swaps.

### CodeRabbit Validation Concern
> The validation check at lines 169-171 verifies that `totalIncrementsInSplits == totalIncrements` (10). With the fix:
> - Routes with errors get 0 increments allocated by DP
> - During validation, routes with 0 increments are skipped (line 142)
> - Only working routes contribute to `totalIncrementsInSplits`
> 
> **Risk**: Valid splits with some erroring routes might be incorrectly rejected if working routes don't absorb all 10 increments.

---

## Pre-Test Setup

1. **Staging URL**: Use the staging frontend pointed at staging SQS
2. **Browser**: Open DevTools → Network tab to inspect `/router/quote` responses
3. **No wallet required**: Quotes work without wallet connection

---

## Test Matrix

### 🔴 Priority 1: Original Bug Reproduction (USDC → BTC)

- [ ] **P1-1**: 400,000 USDC → BTC — Expected: ~4.4 BTC (reasonable output)
- [ ] **P1-2**: 444,000 USDC → BTC — Expected: ~4.8 BTC (threshold amount)
- [ ] **P1-3**: 500,000 USDC → BTC — Expected: **~5.4 BTC** (NOT ~0.4 BTC) ⚠️ **CRITICAL**
- [ ] **P1-4**: 750,000 USDC → BTC — Expected: ~8.1 BTC (scales linearly)
- [ ] **P1-5**: 1,000,000 USDC → BTC — Expected: ~10.8 BTC (large amount)

**Key Verification for P1-3**:
- [ ] Price impact should be < 5% (not 90%+)
- [ ] Route should NOT include Pool 1930 (low-liquidity orderbook)
- [ ] Route SHOULD use high-liquidity CL pools (1943, 2321, etc.)

---

### 🟡 Priority 2: Multi-Asset Pairs (High Liquidity)

#### USDC Pairs
- [ ] **U-1**: 100,000 USDC → OSMO — Expected: Reasonable output, <2% impact
- [ ] **U-2**: 500,000 USDC → OSMO — Expected: Reasonable output, <5% impact
- [ ] **U-3**: 100,000 USDC → ATOM — Expected: Reasonable output, <2% impact
- [ ] **U-4**: 500,000 USDC → ATOM — Expected: Reasonable output, <5% impact
- [ ] **U-5**: 100,000 USDC → ETH — Expected: Reasonable output, <2% impact
- [ ] **U-6**: 500,000 USDC → ETH — Expected: Reasonable output, <5% impact

#### OSMO Pairs
- [ ] **O-1**: 500,000 OSMO → USDC — Expected: Reasonable output
- [ ] **O-2**: 500,000 OSMO → ATOM — Expected: Reasonable output
- [ ] **O-3**: 1,000,000 OSMO → BTC — Expected: Reasonable output

#### Major Crypto Pairs
- [ ] **M-1**: 10 ETH → BTC — Expected: Reasonable output
- [ ] **M-2**: 50 ETH → USDC — Expected: Reasonable output
- [ ] **M-3**: 1 BTC → USDC — Expected: Reasonable output
- [ ] **M-4**: 1 BTC → ETH — Expected: Reasonable output

---

### 🟢 Priority 3: Cosmos Ecosystem Pairs

- [ ] **C-1**: 10,000 TIA → USDC — Expected: Reasonable output
- [ ] **C-2**: 10,000 TIA → OSMO — Expected: Reasonable output
- [ ] **C-3**: 5,000 INJ → USDC — Expected: Reasonable output
- [ ] **C-4**: 5,000 INJ → OSMO — Expected: Reasonable output
- [ ] **C-5**: 10,000 ATOM → TIA — Expected: Reasonable output

---

### 🔵 Priority 4: Edge Cases & Validation Check

These tests specifically verify CodeRabbit's concern about the increment validation.

- [ ] **E-1**: 2,000,000 USDC → BTC (Very large) — Check: Quote succeeds, no validation error
- [ ] **E-2**: 100 USDC → BTC (Small amount) — Check: Quote succeeds (may use single route)
- [ ] **E-3**: 445,000 USDC → BTC (Threshold crossing) — Check: No sudden drop in output
- [ ] **E-4**: 500,000 USDC → TIA (Multi-hop route) — Check: Correct routing through OSMO/ATOM
- [ ] **E-5**: 1,000,000 OSMO → USDC (Orderbook pair) — Check: Should avoid low-liq orderbooks

---

## Detailed Test Procedure

### For Each Test:

1. **Navigate to swap page**:
   ```
   https://stage.osmosis.zone/?from=<FROM_DENOM>&to=<TO_DENOM>
   ```

2. **Enter amount** in the "From" field

3. **Check quote response** in Network tab:
   - Find request to `/router/quote`
   - Verify response includes:
     - `amount_out`: Should be reasonable
     - `price_impact_percentage`: Should be < 5% for large amounts
     - `route`: Check which pools are used

4. **Record results**:
   - Screenshot the swap UI
   - Copy the quote response JSON
   - Note any errors in console

---

## API Direct Testing

You can also test the SQS API directly:

### USDC → BTC (500K) - The Original Bug
```bash
curl "https://sqs-stage.osmosis.zone/router/quote?tokenIn=500000000000ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4&tokenOutDenom=factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC"
```

**Expected**: `amount_out` should be ~5.4 BTC worth (in smallest denomination)

### USDC → OSMO (500K)
```bash
curl "https://sqs-stage.osmosis.zone/router/quote?tokenIn=500000000000ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4&tokenOutDenom=uosmo"
```

---

## Failure Indicators

### ❌ Test FAILS if:
1. **Catastrophic slippage**: Output is 90%+ less than expected
2. **Quote error**: API returns error for valid amounts
3. **Validation rejection**: Error message contains "total increments does not match"
4. **Route includes low-liquidity orderbook**: Pool 1930 appears in route for large BTC swaps
5. **Price impact > 10%**: For liquid pairs with large but reasonable amounts

### ✅ Test PASSES if:
1. Quote returns reasonable output
2. Price impact is proportional to amount
3. High-liquidity pools are preferred
4. No console errors related to routing

---

## Regression Checklist

Before signing off on the fix:

- [ ] **P1-3 (500K USDC → BTC)**: Output is ~5.4 BTC, not ~0.4 BTC
- [ ] **No validation errors**: All quotes succeed without "total increments" error
- [ ] **Routes are sensible**: Large swaps use high-liquidity CL pools
- [ ] **Price impact is reasonable**: < 5% for amounts under $500K in liquid pairs
- [ ] **Edge cases work**: Very small and very large amounts both quote successfully
- [ ] **Multi-hop routes work**: Complex routes (USDC → TIA) work correctly

---

## Test Results Template

```markdown
## Test Run: [DATE]
### Tester: [NAME]
### Environment: [staging URL]
### SQS Version: [commit hash]

### Priority 1 Results:
- [ ] P1-1: 
- [ ] P1-2: 
- [ ] P1-3: 
- [ ] P1-4: 
- [ ] P1-5: 

### Priority 2 Results:
- [ ] U-1 through U-6: 
- [ ] O-1 through O-3: 
- [ ] M-1 through M-4: 

### Priority 3 Results:
- [ ] C-1 through C-5: 

### Priority 4 Results:
- [ ] E-1 through E-5: 

### Issues Found:
- 

### Screenshots/Evidence:
- 
```

---

## Appendix: Pool Reference

| Pool ID | Type | Pair | Liquidity | Notes |
|---------|------|------|-----------|-------|
| 1930 | Orderbook | USDC/BTC | ~$42K | ⚠️ Low liquidity, caused original bug |
| 1943 | CL | allBTC/USDC | ~$1.07M | ✅ High liquidity, preferred |
| 2321 | CL | allBTC/USDC | Variable | ✅ Alternative high-liq pool |

---

## Contact

If tests fail, contact:
- SQS Team: [Slack channel]
- Issue: OSMO-53
