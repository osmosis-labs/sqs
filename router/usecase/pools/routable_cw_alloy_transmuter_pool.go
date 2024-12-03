package pools

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

var _ domain.RoutablePool = &routableAlloyTransmuterPoolImpl{}

type routableAlloyTransmuterPoolImpl struct {
	ChainPool           *cwpoolmodel.CosmWasmPool         "json:\"pool\""
	AlloyTransmuterData *cosmwasmpool.AlloyTransmuterData "json:\"alloy_transmuter_data\""
	Balances            sdk.Coins                         "json:\"balances\""
	TokenInDenom        string                            "json:\"token_in_denom,omitempty\""
	TokenOutDenom       string                            "json:\"token_out_denom,omitempty\""
	TakerFee            osmomath.Dec                      "json:\"taker_fee\""
	SpreadFactor        osmomath.Dec                      "json:\"spread_factor\""
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

	// Check static upper rate limiter
	if err := r.checkStaticRateLimiter(tokenIn); err != nil {
		return osmomath.BigDec{}, err
	}

	tokenInAmount := osmomath.BigDecFromSDKInt(tokenIn.Amount)

	tokenInNormFactorBig := osmomath.NewBigIntFromBigInt(tokenInNormFactor.BigInt())
	tokenOutNormFactorBig := osmomath.NewBigIntFromBigInt(tokenOutNormFactor.BigInt())

	tokenOutAmount := tokenInAmount.MulInt(tokenOutNormFactorBig).QuoInt(tokenInNormFactorBig)

	return tokenOutAmount, nil
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
func (r *routableAlloyTransmuterPoolImpl) checkStaticRateLimiter(tokenInCoin sdk.Coin) error {
	// If no static rate limiter is set, return
	if len(r.AlloyTransmuterData.RateLimiterConfig.StaticLimiterByDenomMap) == 0 {
		return nil
	}

	preComputedData := r.AlloyTransmuterData.PreComputedData
	normalizationFactors := preComputedData.NormalizationScalingFactors

	// Note: -1 for the LP share.
	normalizedBalances := make(map[string]osmomath.Int, len(r.AlloyTransmuterData.AssetConfigs)-1)
	normalizeTotal := osmomath.ZeroInt()

	// Calculate normalized balances
	for i := 0; i < len(r.AlloyTransmuterData.AssetConfigs); i++ {
		assetConfig := r.AlloyTransmuterData.AssetConfigs[i]
		assetDenom := assetConfig.Denom

		// Skip if the asset is alloyed LP share
		if assetDenom == r.AlloyTransmuterData.AlloyedDenom {
			continue
		}

		assetBalance := r.Balances.AmountOf(assetDenom)

		// Add the token in balance to the asset balance
		if assetDenom == tokenInCoin.Denom {
			assetBalance = assetBalance.Add(tokenInCoin.Amount)
		}

		// Subtract the token out balance from the asset balance
		if assetDenom == r.TokenOutDenom {
			assetBalance = assetBalance.Sub(tokenInCoin.Amount)
		}

		normalizationScalingFactor, ok := normalizationFactors[assetDenom]
		if !ok {
			return fmt.Errorf("normalization scaling factor not found for asset %s, pool id %d", assetDenom, r.GetId())
		}

		// Normalize balance
		normalizedBalance := assetBalance.Mul(normalizationScalingFactor)

		// Store normalized balance
		normalizedBalances[assetDenom] = normalizedBalance

		// Update total
		normalizeTotal = normalizeTotal.Add(normalizedBalance)
	}

	// If token in denom is alloyed, we need to validate limiters for all assets' balances except token out.
	// Since the token out composition is decreasing, other assets' weights are increasing.
	// else, we only need to validate the token in denom limiter.
	if tokenInCoin.Denom == r.AlloyTransmuterData.AlloyedDenom {
		for i := 0; i < len(r.AlloyTransmuterData.AssetConfigs); i++ {
			assetConfig := r.AlloyTransmuterData.AssetConfigs[i]
			assetDenom := assetConfig.Denom

			// Skip if the asset is alloyed LP share
			if assetDenom == r.AlloyTransmuterData.AlloyedDenom {
				continue
			}

			// skip if the asset is token out, since its weight is decreasing, no need to check limiter
			if assetDenom == r.TokenOutDenom {
				continue
			}

			// Check if the static rate limiter exists for the asset denom updated balance.
			// If not, continue to the next asset
			staticLimiter, ok := r.AlloyTransmuterData.RateLimiterConfig.GetStaticLimiter(assetDenom)
			if !ok {
				continue
			}

			// Validate upper limit
			upperLimitInt := osmomath.MustNewDecFromStr(staticLimiter.UpperLimit)

			// Asset weight
			assetWeight := normalizedBalances[assetDenom].ToLegacyDec().Quo(normalizeTotal.ToLegacyDec())

			// Check the upper limit
			if assetWeight.GT(upperLimitInt) {
				return domain.StaticRateLimiterInvalidUpperLimitError{
					UpperLimit: staticLimiter.UpperLimit,
					Weight:     assetWeight.String(),
					Denom:      assetDenom,
				}
			}
		}
	} else {
		tokeInStaticLimiter, ok := r.AlloyTransmuterData.RateLimiterConfig.GetStaticLimiter(tokenInCoin.Denom)
		if !ok {
			return nil
		}

		// Validate upper limit
		upperLimitInt := osmomath.MustNewDecFromStr(tokeInStaticLimiter.UpperLimit)

		// Token in weight
		tokenInWeight := normalizedBalances[tokenInCoin.Denom].ToLegacyDec().Quo(normalizeTotal.ToLegacyDec())

		// Check the upper limit
		if tokenInWeight.GT(upperLimitInt) {
			return domain.StaticRateLimiterInvalidUpperLimitError{
				UpperLimit: tokeInStaticLimiter.UpperLimit,
				Weight:     tokenInWeight.String(),
				Denom:      tokenInCoin.Denom,
			}
		}
	}

	return nil
}
