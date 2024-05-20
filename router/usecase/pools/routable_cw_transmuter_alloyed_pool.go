package pools

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v24/x/cosmwasmpool/model"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v24/x/poolmanager/types"
)

var _ sqsdomain.RoutablePool = &routableTransmuterAlloyedPoolImpl{}

type routableTransmuterAlloyedPoolImpl struct {
	ChainPool             *cwpoolmodel.CosmWasmPool        "json:\"pool\""
	TransmuterAlloyedData *sqsdomain.TransmuterAlloyedData "json:\"transmuter_alloyed_data\""
	Balances              sdk.Coins                        "json:\"balances\""
	TokenOutDenom         string                           "json:\"token_out_denom\""
	TakerFee              osmomath.Dec                     "json:\"taker_fee\""
	SpreadFactor          osmomath.Dec                     "json:\"spread_factor\""
}

// GetId implements sqsdomain.RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) GetId() uint64 {
	return r.ChainPool.PoolId
}

// GetPoolDenoms implements sqsdomain.RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) GetPoolDenoms() []string {
	denoms := make([]string, len(r.TransmuterAlloyedData.AssetConfigs))
	for i, config := range r.TransmuterAlloyedData.AssetConfigs {
		denoms[i] = config.Denom
	}
	return denoms
}

// GetType implements sqsdomain.RoutablePool.
func (*routableTransmuterAlloyedPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.CosmWasm
}

// GetSpreadFactor implements sqsdomain.RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

// CalculateTokenOutByTokenIn implements sqsdomain.RoutablePool.
// It calculates the amount of token out given the amount of token in for a transmuter pool.
// Transmuter pool allows no slippage swaps. For v3, the ratio of token in to token out is dependent on the normalization factor.
// Returns error if:
// - the underlying chain pool set on the routable pool is not of transmuter type
// - the token in amount is greater than the balance of the token in
// - the token in amount is greater than the balance of the token out
//
// Note that balance validation does not apply to alloyed asset since it can be minted or burned by the pool.
func (r *routableTransmuterAlloyedPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	tokenOutAmt, err := r.calcTokenOutAmt(tokenIn, r.TokenOutDenom)
	if err != nil {
		return sdk.Coin{}, err
	}

	tokenOutAmtInt := osmomath.NewIntFromBigInt(tokenOutAmt.TruncateInt().BigInt())

	// Validate token out balance if not alloyed
	if r.TokenOutDenom != r.TransmuterAlloyedData.AlloyedDenom {
		if err := validateTransmuterBalance(tokenOutAmtInt, r.Balances, r.TokenOutDenom); err != nil {
			return sdk.Coin{}, err
		}
	}

	return sdk.Coin{Denom: r.TokenOutDenom, Amount: tokenOutAmtInt}, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// String implements sqsdomain.RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d) Transmuter with alloyed denom, pool denoms (%v), token out (%s)", r.ChainPool.PoolId, poolmanagertypes.CosmWasm, r.GetPoolDenoms(), r.TokenOutDenom)
}

// ChargeTakerFeeExactIn implements sqsdomain.RoutablePool.
// Returns tokenInAmount and does not charge any fee for transmuter pools.
func (r *routableTransmuterAlloyedPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (inAmountAfterFee sdk.Coin) {
	return tokenIn
}

// GetTakerFee implements sqsdomain.RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// SetTokenOutDenom implements sqsdomain.RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// CalcSpotPrice implements sqsdomain.RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	return r.calcTokenOutAmt(sdk.Coin{Denom: baseDenom, Amount: osmomath.OneInt()}, quoteDenom)
}

// IsGeneralizedCosmWasmPool implements sqsdomain.RoutablePool.
func (*routableTransmuterAlloyedPoolImpl) IsGeneralizedCosmWasmPool() bool {
	return false
}

// GetCodeID implements sqsdomain.RoutablePool.
func (r *routableTransmuterAlloyedPoolImpl) GetCodeID() uint64 {
	return r.ChainPool.CodeId
}

func (r *routableTransmuterAlloyedPoolImpl) findNormalizationFactors(tokenInDenom, tokenOutDenom string) (osmomath.Int, osmomath.Int, error) {
	tokenInNormalizationFactor := osmomath.Int{}
	tokenOutNormalizationFactor := osmomath.Int{}

	for _, config := range r.TransmuterAlloyedData.AssetConfigs {
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
func (r *routableTransmuterAlloyedPoolImpl) calcTokenOutAmt(tokenIn sdk.Coin, tokenOutDenom string) (osmomath.BigDec, error) {
	tokenInNormFactor, tokenOutNormFactor, err := r.findNormalizationFactors(tokenIn.Denom, tokenOutDenom)
	if err != nil {
		return osmomath.ZeroBigDec(), err
	}

	tokenInAmount := osmomath.NewBigDec(tokenIn.Amount.Int64())

	tokenInNormFactorBig := osmomath.NewBigIntFromBigInt(tokenInNormFactor.BigInt())
	tokenOutNormFactorBig := osmomath.NewBigIntFromBigInt(tokenOutNormFactor.BigInt())

	return tokenInAmount.MulInt(tokenOutNormFactorBig).QuoInt(tokenInNormFactorBig), nil
}
