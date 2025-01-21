package pools

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

var _ domain.RoutablePool = &routableTransmuterPoolImpl{}

type routableTransmuterPoolImpl struct {
	ChainPool     *cwpoolmodel.CosmWasmPool `json:"pool"`
	Balances      sdk.Coins                 `json:"balances"`
	TokenInDenom  string                    `json:"token_in_denom,omitempty"`
	TokenOutDenom string                    `json:"token_out_denom,omitempty"`
	TakerFee      osmomath.Dec              `json:"taker_fee"`
	SpreadFactor  osmomath.Dec              `json:"spread_factor"`
}

// GetId implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) GetId() uint64 {
	return r.ChainPool.PoolId
}

// GetPoolDenoms implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) GetPoolDenoms() []string {
	return r.Balances.Denoms()
}

// GetType implements domain.RoutablePool.
func (*routableTransmuterPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.CosmWasm
}

// GetSpreadFactor implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

// CalculateTokenOutByTokenIn implements domain.RoutablePool.
// It calculates the amount of token out given the amount of token in for a transmuter pool.
// Transmuter pool allows no slippage swaps. It just returns the same amount of token out as token in
// Returns error if:
// - the underlying chain pool set on the routable pool is not of transmuter type
// - the token in amount is greater than the balance of the token in
// - the token in amount is greater than the balance of the token out
func (r *routableTransmuterPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	poolType := r.GetType()

	// Ensure that the pool is concentrated
	if poolType != poolmanagertypes.CosmWasm {
		return sdk.Coin{}, domain.InvalidPoolTypeError{PoolType: int32(poolType)}
	}

	balances := r.Balances

	// Validate token out balance
	if err := validateTransmuterBalance(tokenIn.Amount, balances, r.TokenOutDenom); err != nil {
		return sdk.Coin{}, err
	}

	// No slippage swaps - just return the same amount of token out as token in
	// as long as there is enough liquidity in the pool.
	return sdk.Coin{Denom: r.TokenOutDenom, Amount: tokenIn.Amount}, nil
}

// CalculateTokenInByTokenOut implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) CalculateTokenInByTokenOut(ctx context.Context, tokenOut sdk.Coin) (sdk.Coin, error) {
	poolType := r.GetType()

	// Ensure that the pool is concentrated
	if poolType != poolmanagertypes.CosmWasm {
		return sdk.Coin{}, domain.InvalidPoolTypeError{PoolType: int32(poolType)}
	}

	balances := r.Balances

	// Validate token in balance
	if err := validateTransmuterBalance(tokenOut.Amount, balances, r.TokenInDenom); err != nil {
		return sdk.Coin{}, err
	}

	// No slippage swaps - just return the same amount of token in as token out
	// as long as there is enough liquidity in the pool.
	return sdk.Coin{Denom: r.TokenInDenom, Amount: tokenOut.Amount}, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableTransmuterPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (r *routableTransmuterPoolImpl) GetTokenInDenom() string {
	return r.TokenInDenom
}

// String implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d) Transmuter, pool denoms (%v), token out (%s)", r.ChainPool.PoolId, poolmanagertypes.CosmWasm, r.GetPoolDenoms(), r.TokenOutDenom)
}

// ChargeTakerFeeExactIn implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (inAmountAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// ChargeTakerFeeExactOut implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) ChargeTakerFeeExactOut(tokenIn sdk.Coin) (inAmountAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactOut(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// validateTransmuterBalance validates that the balance of the denom to validate is greater than the token amount.
// Returns nil on success, error otherwise.
func validateTransmuterBalance(tokenAmount osmomath.Int, balances sdk.Coins, denomToValidate string) error {
	balanceToValidate := balances.AmountOf(denomToValidate)
	if tokenAmount.GT(balanceToValidate) {
		return domain.TransmuterInsufficientBalanceError{
			Denom:         denomToValidate,
			BalanceAmount: balanceToValidate.String(),
			Amount:        tokenAmount.String(),
		}
	}

	return nil
}

// GetTakerFee implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// SetTokenInDenom implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) SetTokenInDenom(tokenInDenom string) {
	r.TokenInDenom = tokenInDenom
}

// SetTokenOutDenom implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	return osmomath.OneBigDec(), nil
}

// GetSQSType implements domain.RoutablePool.
func (*routableTransmuterPoolImpl) GetSQSType() domain.SQSPoolType {
	return domain.TransmuterV1
}

// GetCodeID implements domain.RoutablePool.
func (r *routableTransmuterPoolImpl) GetCodeID() uint64 {
	return r.ChainPool.CodeId
}
