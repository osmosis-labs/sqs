package pools

import (
	"context"
	"fmt"
	"strings"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/v30/ingest/types/cosmwasmpool"
	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v30/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v30/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v30/x/poolmanager/types"
)

var _ domain.RoutablePool = &routableAlloyTransmuterPoolImpl{}

const DenomPrefix = "denom::"
const AssetGroupPrefix = "asset_group::"

type routableAlloyTransmuterPoolImpl struct {
	ChainPool           *cwpoolmodel.CosmWasmPool         `json:"pool"`
	AlloyTransmuterData *cosmwasmpool.AlloyTransmuterData `json:"alloy_transmuter_data"`
	Balances            sdk.Coins                         `json:"balances"`
	TokenInDenom        string                            `json:"token_in_denom,omitempty"`
	TokenOutDenom       string                            `json:"token_out_denom,omitempty"`
	TakerFee            osmomath.Dec                      `json:"taker_fee"`
	SpreadFactor        osmomath.Dec                      `json:"spread_factor"`
	LiquidityCap        osmomath.Int                      `json:"liquidity_cap"`
}

// GetId implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) GetId() uint64 {
	return r.ChainPool.PoolId
}

// GetPoolDenoms implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) GetPoolDenoms() []string {
	denoms := make([]string, len(r.AlloyTransmuterData.AssetConfigs))
	for i, config := range r.AlloyTransmuterData.AssetConfigs {
		denoms[i] = config.Denom
	}
	return denoms
}

// GetType implements domain.RoutablePool.
func (*routableAlloyTransmuterPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.CosmWasm
}

// GetSpreadFactor implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

// GetLiquidityCap implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) GetLiquidityCap() osmomath.Int {
	return r.LiquidityCap
}

// CalculateTokenOutByTokenIn implements domain.RoutablePool.
// It calculates the amount of token out given the amount of token in for a transmuter pool.
// Transmuter pool allows no slippage swaps. For v3, the ratio of token in to token out is dependent on the normalization factor.
// Returns error if:
// - the underlying chain pool set on the routable pool is not of transmuter type
// - the token in amount is greater than the balance of the token in
// - the token in amount is greater than the balance of the token out
//
// Note that balance validation does not apply to alloyed asset since it can be minted or burned by the pool.
func (r *routableAlloyTransmuterPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	tokenOutAmt, err := r.CalcTokenOutAmt(tokenIn, r.TokenOutDenom)
	if err != nil {
		return sdk.Coin{}, err
	}

	tokenOutAmtInt := tokenOutAmt.Dec().TruncateInt()

	// Validate token out balance if not alloyed
	if r.TokenOutDenom != r.AlloyTransmuterData.AlloyedDenom {
		if err := validateTransmuterBalance(tokenOutAmtInt, r.Balances, r.TokenOutDenom); err != nil {
			return sdk.Coin{}, err
		}
	}

	return sdk.Coin{Denom: r.TokenOutDenom, Amount: tokenOutAmtInt}, nil
}

// CalculateTokenInByTokenOut implements domain.RoutablePool.
// It calculates the amount of token in given the amount of token out for a transmuter pool.
// Transmuter pool allows no slippage swaps. For v3, the ratio of token out to token in is dependent on the normalization factor.
// Returns error if:
// - the underlying chain pool set on the routable pool is not of transmuter type
// - the token out amount is greater than the balance of the token out
// - the token out amount is greater than the balance of the token in
//
// Note that balance validation does not apply to alloyed asset since it can be minted or burned by the pool.
func (r *routableAlloyTransmuterPoolImpl) CalculateTokenInByTokenOut(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	tokenInAmt, err := r.CalcTokenInAmt(tokenIn, r.TokenOutDenom)
	if err != nil {
		return sdk.Coin{}, err
	}

	tokenInAmtInt := tokenInAmt.Dec().TruncateInt()

	// Validate token out balance if not alloyed
	if r.TokenInDenom != r.AlloyTransmuterData.AlloyedDenom {
		if err := validateTransmuterBalance(tokenInAmtInt, r.Balances, r.TokenInDenom); err != nil {
			return sdk.Coin{}, err
		}
	}

	return sdk.Coin{Denom: r.TokenInDenom, Amount: tokenInAmtInt}, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) GetTokenInDenom() string {
	return r.TokenInDenom
}

// String implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d) Transmuter with alloyed denom, pool denoms (%v), token out (%s)", r.ChainPool.PoolId, poolmanagertypes.CosmWasm, r.GetPoolDenoms(), r.TokenOutDenom)
}

// ChargeTakerFeeExactIn implements domain.RoutablePool.
// Returns tokenInAmount and does not charge any fee for transmuter pools.
func (r *routableAlloyTransmuterPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (inAmountAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// ChargeTakerFeeExactOut implements domain.RoutablePool.
// Returns tokenInAmount and does not charge any fee for transmuter pools.
func (r *routableAlloyTransmuterPoolImpl) ChargeTakerFeeExactOut(tokenIn sdk.Coin) (inAmountAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactOut(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// GetTakerFee implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// SetTokenInDenom implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) SetTokenInDenom(tokenInDenom string) {
	r.TokenInDenom = tokenInDenom
}

// SetTokenOutDenom implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	return r.CalcTokenOutAmt(sdk.Coin{Denom: baseDenom, Amount: osmomath.OneInt()}, quoteDenom)
}

// GetSQSType implements domain.RoutablePool.
func (*routableAlloyTransmuterPoolImpl) GetSQSType() domain.SQSPoolType {
	return domain.AlloyedTransmuter
}

// GetCodeID implements domain.RoutablePool.
func (r *routableAlloyTransmuterPoolImpl) GetCodeID() uint64 {
	return r.ChainPool.CodeId
}

// FindNormalizationFactors finds the normalization factors for the given token in and token out denoms.
// It is required for calculating token out & spot price.
// For more information about normalization factor, please refer to [transmuter documentation](https://github.com/osmosis-labs/transmuter/tree/v3.0.0?tab=readme-ov-file#normalization-factors).
func (r *routableAlloyTransmuterPoolImpl) FindNormalizationFactors(tokenInDenom, tokenOutDenom string) (osmomath.Int, osmomath.Int, error) {
	tokenInNormalizationFactor := osmomath.Int{}
	tokenOutNormalizationFactor := osmomath.Int{}

	for _, config := range r.AlloyTransmuterData.AssetConfigs {
		if config.Denom == tokenInDenom {
			tokenInNormalizationFactor = config.NormalizationFactor
		}

		if config.Denom == tokenOutDenom {
			tokenOutNormalizationFactor = config.NormalizationFactor
		}

		if !tokenInNormalizationFactor.IsNil() && !tokenOutNormalizationFactor.IsNil() {
			break
		}
	}

	if tokenInNormalizationFactor.IsNil() {
		return tokenInNormalizationFactor, tokenOutNormalizationFactor, domain.MissingNormalizationFactorError{Denom: tokenInDenom, PoolId: r.GetId()}
	}

	if tokenOutNormalizationFactor.IsNil() {
		return tokenInNormalizationFactor, tokenOutNormalizationFactor, domain.MissingNormalizationFactorError{Denom: tokenOutDenom, PoolId: r.GetId()}
	}

	return tokenInNormalizationFactor, tokenOutNormalizationFactor, nil
}

// Calculate the token out amount based on the normalization factors:
//
// token_out_amt / token_out_norm_factor = token_in_amt / token_in_norm_factor
// token_out_amt = token_in_amt * token_out_norm_factor / token_in_norm_factor
func (r *routableAlloyTransmuterPoolImpl) CalcTokenOutAmt(tokenIn sdk.Coin, tokenOutDenom string) (osmomath.BigDec, error) {
	tokenInNormFactor, tokenOutNormFactor, err := r.FindNormalizationFactors(tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	if tokenInNormFactor.IsZero() {
		return osmomath.BigDec{}, domain.ZeroNormalizationFactorError{Denom: tokenIn.Denom, PoolId: r.GetId()}
	}

	if tokenOutNormFactor.IsZero() {
		return osmomath.BigDec{}, domain.ZeroNormalizationFactorError{Denom: tokenOutDenom, PoolId: r.GetId()}
	}

	tokenInAmount := osmomath.BigDecFromSDKInt(tokenIn.Amount)

	tokenInNormFactorBig := osmomath.NewBigIntFromBigInt(tokenInNormFactor.BigInt())
	tokenOutNormFactorBig := osmomath.NewBigIntFromBigInt(tokenOutNormFactor.BigInt())

	tokenOutAmount := tokenInAmount.MulInt(tokenOutNormFactorBig).QuoInt(tokenInNormFactorBig)
	tokenOut := sdk.NewCoin(tokenOutDenom, tokenOutAmount.Dec().TruncateInt())

	// Compute adjustment using rebalancing configs and normalization scaling factors.
	// Build balances after swap safely: do not add/subtract alloyed LP share, and do not underflow non-alloyed.
	balancesAfter := r.Balances
	if tokenIn.Denom != r.AlloyTransmuterData.AlloyedDenom {
		balancesAfter = balancesAfter.Add(tokenIn)
	}

	if tokenOut.Denom != r.AlloyTransmuterData.AlloyedDenom {
		if tokenOut.Amount.GT(balancesAfter.AmountOf(tokenOut.Denom)) {
			return osmomath.BigDec{}, domain.TransmuterInsufficientBalanceError{Denom: tokenOut.Denom, BalanceAmount: balancesAfter.AmountOf(tokenOut.Denom).String(), Amount: tokenOut.Amount.String()}
		}
		balancesAfter = balancesAfter.Sub(sdk.NewCoin(tokenOut.Denom, tokenOut.Amount))
	}

	totalAdjustmentRate, scaler, err := r.computeTotalAdjustmentRate(r.Balances, balancesAfter)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	if !totalAdjustmentRate.IsZero() {
		denomScaler, ok := r.AlloyTransmuterData.PreComputedData.NormalizationScalingFactors[tokenOutDenom]
		if !ok || denomScaler.IsZero() {
			return osmomath.BigDec{}, domain.MissingNormalizationFactorError{Denom: tokenOutDenom, PoolId: r.GetId()}
		}

		totalAdjustment := totalAdjustmentRate.Mul(scaler.ToLegacyDec()).Quo(math.LegacyDec(denomScaler))

		if totalAdjustmentRate.IsPositive() {
			tokenOutAmount = tokenOutAmount.Add(osmomath.BigDecFromSDKInt(totalAdjustment.TruncateInt()))
		}

		if totalAdjustmentRate.IsNegative() {
			tokenOutAmount = tokenOutAmount.Add(osmomath.BigDecFromSDKInt(totalAdjustment.Ceil().TruncateInt()))
		}
	}

	// Check static upper rate limiter
	tokenOut = sdk.NewCoin(tokenOutDenom, tokenOutAmount.Dec().TruncateInt())
	if err := r.checkStaticRateLimiter(tokenIn, tokenOut); err != nil {
		return osmomath.BigDec{}, err
	}

	return tokenOutAmount, nil
}

// Calculate the token in amount based on the normalization factors:
//
// token_in_amt = token_out_amt * token_in_norm_factor / token_out_norm_factor
func (r *routableAlloyTransmuterPoolImpl) CalcTokenInAmt(tokenOut sdk.Coin, tokenInDenom string) (osmomath.BigDec, error) {
	tokenInNormFactor, tokenOutNormFactor, err := r.FindNormalizationFactors(tokenInDenom, tokenOut.Denom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	if tokenInNormFactor.IsZero() {
		return osmomath.BigDec{}, domain.ZeroNormalizationFactorError{Denom: tokenOut.Denom, PoolId: r.GetId()}
	}

	if tokenOutNormFactor.IsZero() {
		return osmomath.BigDec{}, domain.ZeroNormalizationFactorError{Denom: tokenInDenom, PoolId: r.GetId()}
	}

	tokenOutAmount := osmomath.BigDecFromSDKInt(tokenOut.Amount)

	tokenOutNormFactorBig := osmomath.NewBigIntFromBigInt(tokenOutNormFactor.BigInt())
	tokenInNormFactorBig := osmomath.NewBigIntFromBigInt(tokenInNormFactor.BigInt())

	tokenInAmount := tokenOutAmount.MulInt(tokenInNormFactorBig).QuoInt(tokenOutNormFactorBig)

	// Check static upper rate limiter
	tokenIn := sdk.NewCoin(tokenInDenom, tokenInAmount.Dec().TruncateInt())
	if err := r.checkStaticRateLimiter(tokenIn, tokenOut); err != nil {
		return osmomath.BigDec{}, err
	}

	return tokenInAmount, nil
}

func (r *routableAlloyTransmuterPoolImpl) computeNormalizedBalances(balances sdk.Coins) (map[string]osmomath.Int, osmomath.Int, error) {
	preComputedData := r.AlloyTransmuterData.PreComputedData
	normalizationFactors := preComputedData.NormalizationScalingFactors

	// Note: -1 for the LP share.
	normalizedBalances := make(map[string]osmomath.Int, len(r.AlloyTransmuterData.AssetConfigs)-1)
	normalizedTotal := osmomath.ZeroInt()

	// Calculate normalized balances
	for i := 0; i < len(r.AlloyTransmuterData.AssetConfigs); i++ {
		assetConfig := r.AlloyTransmuterData.AssetConfigs[i]
		assetDenom := assetConfig.Denom

		// Skip if the asset is alloyed LP share
		if assetDenom == r.AlloyTransmuterData.AlloyedDenom {
			continue
		}

		assetBalance := balances.AmountOf(assetDenom)

		normalizationScalingFactor, ok := normalizationFactors[assetDenom]
		if !ok {
			return nil, osmomath.ZeroInt(), domain.MissingNormalizationFactorError{Denom: assetDenom, PoolId: r.GetId()}
		}

		// Normalize balance
		normalizedBalance := assetBalance.Mul(normalizationScalingFactor)

		// Store normalized balance
		normalizedBalances[assetDenom] = normalizedBalance

		// Update total
		normalizedTotal = normalizedTotal.Add(normalizedBalance)
	}

	return normalizedBalances, normalizedTotal, nil
}

// checkStaticRateLimiter checks the static rate limiter.
// If token in denom is not alloyed, we only need to validate the token in balance.
// Since the token in balance is the only one that is increased by the current quote.
//
// If token in denom is alloyed, we need to validate all assets' balances except token out.
// Since the token out composition is decreasing, other assets' weights are increasing.
//
// Note: static rate limit only has an upper limit.
// No-op if the static rate limiter is not set.
// Returns error if the token in weight is greater than the upper limit.
// Returns nil if the token in weight is less than or equal to the upper limit.
func (r *routableAlloyTransmuterPoolImpl) checkStaticRateLimiter(tokenInCoin sdk.Coin, tokenOutCoin sdk.Coin) error {
	// If no rebalancing config is set, return
	if len(r.AlloyTransmuterData.RebalancingConfigs) == 0 {
		return nil
	}

	balances := r.Balances.Add(tokenInCoin)
	if tokenOutCoin.Denom != r.AlloyTransmuterData.AlloyedDenom {
		currentOutBal := balances.AmountOf(tokenOutCoin.Denom)
		subAmount := tokenOutCoin.Amount
		if subAmount.GT(currentOutBal) {
			subAmount = currentOutBal
		}
		if !subAmount.IsZero() {
			balances = balances.Sub(sdk.NewCoin(tokenOutCoin.Denom, subAmount))
		}
	}

	normalizedBalances, normalizedTotal, err := r.computeNormalizedBalances(balances)
	if err != nil {
		return err
	}

	// If token in denom is alloyed, we need to validate limiters for all assets' balances except token out.
	// Since the token out composition is decreasing, other assets' weights are increasing.
	// else, we only need to validate the token in denom limiter.

	for scope, rebalancingConfig := range r.AlloyTransmuterData.RebalancingConfigs {

		// skip if the asset is token out, since its weight is decreasing, no need to check limiter
		if scope == DenomPrefix+r.TokenOutDenom {
			continue
		}

		// Check if the static rate limiter exists for the asset denom updated balance.
		// If not, continue to the next asset
		if rebalancingConfig.Limit == "" {
			continue
		}

		// Validate upper limit
		upperLimitInt := osmomath.MustNewDecFromStr(rebalancingConfig.Limit)

		scopeWeight := osmomath.ZeroDec()

		// if it's asset assetGroup, sum weight for all under assetGroup
		if assetGroup, ok := r.AlloyTransmuterData.AssetGroups[strings.TrimPrefix(scope, AssetGroupPrefix)]; ok {
			for _, denom := range assetGroup.Denoms {
				scopeWeight = scopeWeight.Add(normalizedBalances[denom].ToLegacyDec().Quo(normalizedTotal.ToLegacyDec()))
			}
		} else {
			// if it's single denom, use the weight for the denom
			scopeWeight = normalizedBalances[strings.TrimPrefix(scope, DenomPrefix)].ToLegacyDec().Quo(normalizedTotal.ToLegacyDec())
		}

		// Check the upper limit
		if scopeWeight.GT(upperLimitInt) {
			return domain.StaticRateLimiterInvalidUpperLimitError{
				UpperLimit: rebalancingConfig.Limit,
				Weight:     scopeWeight.String(),
				Scope:      scope,
			}
		}
	}

	return nil
}

// positive(x) = max(x, 0)
func positive(x osmomath.Dec) osmomath.Dec {
	if x.IsNegative() {
		return osmomath.ZeroDec()
	}
	return x
}

// max(a,b)
func dmax(a, b osmomath.Dec) osmomath.Dec {
	if a.GTE(b) {
		return a
	}
	return b
}

// min(a,b)
func dmin(a, b osmomath.Dec) osmomath.Dec {
	if a.LTE(b) {
		return a
	}
	return b
}

// weightedDistance W(x) from the ideal band with piecewise rates.
//
//	W(x) = rc*(kL - x)_+
//	     + rs*(phiL - max{x,kL})_+
//	     + rs*(min{x,kU} - phiU)_+
//	     + rc*(x - kU)_+
//
// W(x) is area under curve of the following graph:
//
// rate(x)
//
//	^
//	| rc ┌───────┐                   ┌───────┐
//	|    │       │                   │       │
//	|    │       │                   │       │
//	| rs │       └────────┐  ┌───────┘       │
//	|    │                │  │               │
//	|  0 └────────────────┴──┴───────────────┴─────────> x
//	|    0      kL       φL  φU     kU       δ(limit)
//
// where x is the %balance of the asset in the liquidity pool.
//
// zones:
//
//	[0, kL)     → rate = rc  (critical low)
//	[kL, φL)    → rate = rs  (strained low)
//	[φL, φU]    → rate = 0   (ideal band)
//	(φU, kU]    → rate = rs  (strained high)
//	(kU, δ]     → rate = rc  (critical high)
func weightedDistance(x osmomath.Dec, p cosmwasmpool.RebalancingConfig) osmomath.Dec {
	criticalLower := osmomath.MustNewDecFromStr(p.CriticalLower)
	idealLower := osmomath.MustNewDecFromStr(p.IdealLower)
	criticalUpper := osmomath.MustNewDecFromStr(p.CriticalUpper)
	idealUpper := osmomath.MustNewDecFromStr(p.IdealUpper)

	rc := osmomath.MustNewDecFromStr(p.AdjustmentRateCritical)
	rs := osmomath.MustNewDecFromStr(p.AdjustmentRateStrained)

	criticalLow := rc.Mul(positive(criticalLower.Sub(x)))
	strainedLow := rs.Mul(positive(idealLower.Sub(dmax(x, criticalLower))))
	strainedHigh := rs.Mul(positive(dmin(x, criticalUpper).Sub(idealUpper)))
	criticalHigh := rc.Mul(positive(x.Sub(criticalUpper)))

	return criticalLow.Add(strainedLow).Add(strainedHigh).Add(criticalHigh)
}

func (r *routableAlloyTransmuterPoolImpl) computeTotalAdjustmentRate(balanceBefore, balanceAfter sdk.Coins) (osmomath.Dec, osmomath.Int, error) {
	totalAdjustmentRate := osmomath.ZeroDec()
	normalizedBalanceBefore, normalizedTotalBefore, err := r.computeNormalizedBalances(balanceBefore)
	if err != nil {
		return osmomath.ZeroDec(), osmomath.ZeroInt(), err
	}

	normalizedBalanceAfter, normalizedTotalAfter, err := r.computeNormalizedBalances(balanceAfter)
	if err != nil {
		return osmomath.ZeroDec(), osmomath.ZeroInt(), err
	}

	for scope, rebalancingConfig := range r.AlloyTransmuterData.RebalancingConfigs {
		scopeBalanceBefore := osmomath.ZeroInt()
		scopeBalanceAfter := osmomath.ZeroInt()

		// if it's asset group, sum weight for all under group
		if assetGroupLabel, found := strings.CutPrefix(scope, AssetGroupPrefix); found {
			for _, denom := range r.AlloyTransmuterData.AssetGroups[assetGroupLabel].Denoms {
				scopeBalanceBefore = scopeBalanceBefore.Add(normalizedBalanceBefore[denom])
				scopeBalanceAfter = scopeBalanceAfter.Add(normalizedBalanceAfter[denom])
			}
		}

		// if it's single denom, use the weight for the denom
		if denom, found := strings.CutPrefix(scope, DenomPrefix); found {
			scopeBalanceBefore = normalizedBalanceBefore[denom]
			scopeBalanceAfter = normalizedBalanceAfter[denom]
		}

		scopeWeightBefore := scopeBalanceBefore.ToLegacyDec().Quo(normalizedTotalBefore.ToLegacyDec())
		scopeWeightAfter := scopeBalanceAfter.ToLegacyDec().Quo(normalizedTotalAfter.ToLegacyDec())

		adjustmentRate := weightedDistance(scopeWeightBefore, rebalancingConfig).Sub(weightedDistance(scopeWeightAfter, rebalancingConfig))
		totalAdjustmentRate = totalAdjustmentRate.Add(adjustmentRate)
	}

	scaler := osmomath.ZeroInt()

	if totalAdjustmentRate.IsPositive() { // helpful swap
		scaler = normalizedTotalBefore
	} else if totalAdjustmentRate.IsNegative() { // harmful swap
		scaler = osmomath.MaxInt(normalizedTotalBefore, normalizedTotalAfter)
	}

	return totalAdjustmentRate, scaler, nil
}

// TODO:
// - [x] calculate balance change for a swap (extract check limit logic out, generalize into balance changes)
// - [x] Find sum of all W(b') - W(b) and scale it by:
// 1.  **If the swap is beneficial (results in an incentive):** The calculation is scaled by `B_total_before`.
// -   **Justification**: Incentives are paid from a pool of previously collected fees. It is logical to scale the reward based on the state of the pool *before* the user's helpful contribution. This provides a fair reward relative to the pool's history and prevents a single large, helpful deposit from draining a disproportionate amount of the incentive fund.
// 2.  **If the swap is harmful (results in a fee):** The calculation is scaled by `max(B_total_before, B_total_after)`.
//     -   **Justification**: This provides maximum security for the pool.
//         -   For **harmful joins** (liquidity addition), the fee is based on `B_total_after`. This ensures the penalty is proportional to the new, larger pool size that the user has unbalanced.
//         -   For **harmful exits** (liquidity withdrawal), the fee is based on `B_total_before`. This ensures the penalty is proportional to the state of the pool *before* it was damaged by the withdrawal.
//
// - [x] this will resulted in fee / incentive to distribute
// - [x] handle denom/asset_group split
// - [x] ingest incentive pool info
// - [ ] handle unhealthy incentive pool
// - [ ] test first order fee / incentive case -- today's target
// - [ ] now we have to find if we need second order incentive swap
