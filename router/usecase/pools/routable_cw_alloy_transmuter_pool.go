package pools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/rustffi"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v25/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
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

const (
	alloyedLPShareDenomComponent = "all"
)

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

	// Check static & change upper rate limiter
	// We only need to check it for the token in coin since that is the only one that is increased by the current quote.
	if err := r.checkStaticRateLimiter(tokenIn); err != nil {
		return osmomath.BigDec{}, err
	}
	if err := r.checkChangeRateLimiter(tokenIn, time.Now()); err != nil {
		return osmomath.BigDec{}, err
	}

	tokenInAmount := osmomath.BigDecFromSDKInt(tokenIn.Amount)

	tokenInNormFactorBig := osmomath.NewBigIntFromBigInt(tokenInNormFactor.BigInt())
	tokenOutNormFactorBig := osmomath.NewBigIntFromBigInt(tokenOutNormFactor.BigInt())

	tokenOutAmount := tokenInAmount.MulInt(tokenOutNormFactorBig).QuoInt(tokenInNormFactorBig)

	return tokenOutAmount, nil
}

// checkStaticRateLimiter checks the static rate limiter for the token in coin.
// Note: static rate limit only has an upper limit.
// Therefore, we only need to validate the token in balance.
// No-op if the static rate limiter is not set.
// Returns error if the token in weight is greater than the upper limit.
// Returns nil if the token in weight is less than or equal to the upper limit.
func (r *routableAlloyTransmuterPoolImpl) checkStaticRateLimiter(tokenInCoin sdk.Coin) error {
	// If no static rate limiter is set, return
	if len(r.AlloyTransmuterData.RateLimiterConfig.StaticLimiterByDenomMap) == 0 {
		return nil
	}

	// Check if the static rate limiter exists for the token in denom updated balance.
	tokenInStaticLimiter, ok := r.AlloyTransmuterData.RateLimiterConfig.GetStaticLimiter(tokenInCoin.Denom)
	if !ok {
		return nil
	}

	weights, err := r.computeResultedWeights(tokenInCoin)
	if err != nil {
		return err
	}

	// Validate upper limit
	upperLimitInt := osmomath.MustNewDecFromStr(tokenInStaticLimiter.UpperLimit)

	// Token in weight
	tokenInWeight := weights[tokenInCoin.Denom]

	// Check the upper limit
	if tokenInWeight.GT(upperLimitInt) {
		return domain.StaticRateLimiterInvalidUpperLimitError{
			UpperLimit: tokenInStaticLimiter.UpperLimit,
			Weight:     tokenInWeight.String(),
			Denom:      tokenInCoin.Denom,
		}
	}

	return nil
}

func (r *routableAlloyTransmuterPoolImpl) checkChangeRateLimiter(tokenInCoin sdk.Coin, time time.Time) error {
	tokenInChangeLimiter, ok := r.AlloyTransmuterData.RateLimiterConfig.GetChangeLimiter(tokenInCoin.Denom)

	// no error if rate limiter not found
	if !ok {
		return nil
	}

	latestRemovedDivision, updatedDivisions, err := cleanUpOutdatedDivision(tokenInChangeLimiter, time)
	if err != nil {
		return err
	}

	// Check for upper limit if there is any existing division or there is any removed divisions
	hasAnyPrevDataPoints := latestRemovedDivision != nil || len(updatedDivisions) != 0

	if hasAnyPrevDataPoints {
		latestValue, err := osmomath.NewDecFromStr(latestRemovedDivision.LatestValue)
		if err != nil {
			return err
		}
		integral, err := osmomath.NewDecFromStr(latestRemovedDivision.Integral)
		if err != nil {
			return err
		}
		ffiLatestRemovedDivision, err := rustffi.NewFFIDivisionRaw(
			latestRemovedDivision.StartedAt, latestRemovedDivision.UpdatedAt, latestValue, integral,
		)
		if err != nil {
			return err
		}

		// TODO: Find a way to remove this interim type
		type _division = struct {
			StartedAt   uint64
			UpdatedAt   uint64
			LatestValue string
			Integral    string
		}

		_updatedDivisions := make([]_division, len(updatedDivisions))
		for _, division := range updatedDivisions {
			_updatedDivisions = append(_updatedDivisions, _division(division))
		}

		ffiUpdatedDivisions, err := rustffi.NewFFIDivisionsRaw(_updatedDivisions)
		if err != nil {
			return err
		}

		divisionSize := tokenInChangeLimiter.WindowConfig.WindowSize / tokenInChangeLimiter.WindowConfig.DivisionCount

		avg, err := rustffi.CompressedMovingAverage(&ffiLatestRemovedDivision, ffiUpdatedDivisions, divisionSize, tokenInChangeLimiter.WindowConfig.WindowSize, uint64(time.UnixNano()))
		if err != nil {
			return err
		}

		// Calculate upper limit using saturating addition
		boundaryOffset, err := osmomath.NewDecFromStr(tokenInChangeLimiter.BoundaryOffset)
		if err != nil {
			return err
		}

		upperLimit := avg.Add(boundaryOffset)
		weights, err := r.computeResultedWeights(tokenInCoin)
		if err != nil {
			return err
		}

		// Check if the value exceeds the upper limit
		tokenInWeight := weights[tokenInCoin.Denom]
		if tokenInWeight.GT(upperLimit) {
			return domain.StaticRateLimiterInvalidUpperLimitError{
				UpperLimit: upperLimit.String(),
				Weight:     tokenInWeight.String(),
				Denom:      tokenInCoin.Denom,
			}
		}

	}

	return nil
}

// cleanUpOutdatedDivision checks if any divisions in the change limiter is out of the interested window given a specified time
// returns (latestRemovedDivision, updatedDivisions, error)
//
// CONTRACT: Divisions must be ordered by `StartedAt`
func cleanUpOutdatedDivision(changeLimier cosmwasmpool.ChangeLimiter, time time.Time) (*cosmwasmpool.Division, []cosmwasmpool.Division, error) {
	divisions := changeLimier.Divisions
	windowSize := changeLimier.WindowConfig.WindowSize
	divisionSize := changeLimier.WindowConfig.WindowSize / changeLimier.WindowConfig.DivisionCount

	var latestRemovedDivision *cosmwasmpool.Division
	latestRemovedIndex := -1

	for i, division := range divisions {
		latestValue, err := osmomath.NewDecFromStr(division.LatestValue)
		if err != nil {
			return nil, []cosmwasmpool.Division{}, err
		}
		integral, err := osmomath.NewDecFromStr(division.Integral)
		if err != nil {
			return nil, []cosmwasmpool.Division{}, err
		}

		ffiDivision, err := rustffi.NewFFIDivisionRaw(division.StartedAt, division.UpdatedAt, latestValue, integral)
		if err != nil {
			return nil, []cosmwasmpool.Division{}, err
		}
		isDivisionOutdated, err := rustffi.IsDivisionOutdated(ffiDivision, uint64(time.UnixNano()), windowSize, divisionSize)
		if err != nil {
			return nil, []cosmwasmpool.Division{}, err
		}

		if isDivisionOutdated {
			latestRemovedDivision = &division
			latestRemovedIndex = i
		} else {
			break
		}
	}

	// no division is outdated or no division at all
	if latestRemovedDivision == nil || len(divisions) == 0 {
		return nil, divisions, nil
	}

	// every division is outdated
	if latestRemovedIndex == len(divisions)-1 {
		return latestRemovedDivision, []cosmwasmpool.Division{}, nil
	}

	// some division before last division is outdated
	return latestRemovedDivision, divisions[latestRemovedIndex+1:], nil
}

func (r *routableAlloyTransmuterPoolImpl) computeResultedWeights(tokenInCoin sdk.Coin) (map[string]osmomath.Dec, error) {
	preComputedData := r.AlloyTransmuterData.PreComputedData
	normalizationFactors := preComputedData.NormalizationScalingFactors

	// Note: -1 for the LP share.
	normalizedBalances := make(map[string]osmomath.Int, len(r.AlloyTransmuterData.AssetConfigs)-1)
	normalizeTotal := osmomath.ZeroInt()

	// Calculate normalized balances
	for i := 0; i < len(r.AlloyTransmuterData.AssetConfigs); i++ {
		assetConfig := r.AlloyTransmuterData.AssetConfigs[i]
		assetDenom := assetConfig.Denom

		// Skip if the asset is alloyed LP hsare
		if strings.Contains(assetDenom, alloyedLPShareDenomComponent) {
			continue
		}

		assetBalance := r.Balances.AmountOf(assetDenom)

		// Add the token in balance to the asset balance
		if assetDenom == tokenInCoin.Denom {
			assetBalance = assetBalance.Add(tokenInCoin.Amount)
		}

		normalizationScalingFactor, ok := normalizationFactors[assetDenom]
		if !ok {
			return nil, fmt.Errorf("normalization scaling factor not found for asset %s, pool id %d", assetDenom, r.GetId())
		}

		// Normalize balance
		normalizedBalance := assetBalance.Mul(normalizationScalingFactor)

		// Store normalized balance
		normalizedBalances[assetDenom] = normalizedBalance

		// Update total
		normalizeTotal = normalizeTotal.Add(normalizedBalance)
	}

	// Calculate weights
	// Note: -1 for the alloyed LP share.
	weights := make(map[string]osmomath.Dec, len(r.AlloyTransmuterData.AssetConfigs)-1)
	for i := 0; i < len(r.AlloyTransmuterData.AssetConfigs); i++ {
		assetConfig := r.AlloyTransmuterData.AssetConfigs[i]
		assetDenom := assetConfig.Denom

		// Skip if the asset is alloyed LP hsare
		if strings.Contains(assetDenom, alloyedLPShareDenomComponent) {
			continue
		}

		// Calculate weight
		weights[assetDenom] = normalizedBalances[assetDenom].ToLegacyDec().Quo(normalizeTotal.ToLegacyDec())
	}

	return weights, nil
}
