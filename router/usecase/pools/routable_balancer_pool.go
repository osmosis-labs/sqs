package pools

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"

	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
)

var _ domain.RoutablePool = &routableBalancerPoolImpl{}

type routableBalancerPoolImpl struct {
	ChainPool     *balancer.Pool `json:"pool"`
	TokenInDenom  string         `json:"token_in_denom,omitempty"`
	TokenOutDenom string         `json:"token_out_denom,omitempty"`
	TakerFee      osmomath.Dec   `json:"taker_fee"`
}

// CalculateTokenOutByTokenIn implements RoutablePool.
func (r *routableBalancerPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	tokenOut, err := r.ChainPool.CalcOutAmtGivenIn(sdk.Context{}.WithContext(ctx), sdk.Coins{tokenIn}, r.TokenOutDenom, r.GetSpreadFactor())
	if err != nil {
		return sdk.Coin{}, err
	}

	return tokenOut, nil
}

// CalculateTokenInByTokenOut implements RoutablePool.
func (r *routableBalancerPoolImpl) CalculateTokenInByTokenOut(ctx context.Context, tokenOut sdk.Coin) (sdk.Coin, error) {
	tokenIn, err := r.ChainPool.CalcInAmtGivenOut(sdk.Context{}.WithContext(ctx), sdk.Coins{tokenOut}, r.TokenInDenom, r.GetSpreadFactor())
	if err != nil {
		return sdk.Coin{}, err
	}

	return tokenIn, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableBalancerPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (r *routableBalancerPoolImpl) GetTokenInDenom() string {
	return r.TokenInDenom
}

// String implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d), pool denoms (%v), token out (%s)", r.ChainPool.Id, poolmanagertypes.Balancer, r.ChainPool.GetPoolDenoms(sdk.Context{}), r.TokenOutDenom)
}

// ChargeTakerFee implements domain.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (r *routableBalancerPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.TakerFee)
	return tokenInAfterTakerFee
}

// ChargeTakerFee implements domain.RoutablePool.
// Charges the taker fee for the given token out and returns the token out after the fee has been charged.
func (r *routableBalancerPoolImpl) ChargeTakerFeeExactOut(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactOut(tokenIn, r.TakerFee)
	return tokenInAfterTakerFee
}

// GetTakerFee implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// SetTokenInDenom implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) SetTokenInDenom(tokenInDenom string) {
	r.TokenInDenom = tokenInDenom
}

// SetTokenOutDenom implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// GetSpreadFactor implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.ChainPool.GetSpreadFactor(sdk.Context{})
}

// GetId implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) GetId() uint64 {
	return r.ChainPool.Id
}

// GetPoolDenoms implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) GetPoolDenoms() []string {
	return r.ChainPool.GetPoolDenoms(sdk.Context{})
}

// GetType implements domain.RoutablePool.
func (*routableBalancerPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.Balancer
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	spotPrice, err := r.ChainPool.SpotPrice(sdk.Context{}.WithContext(ctx), quoteDenom, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}
	return spotPrice, nil
}

// GetSQSType implements domain.RoutablePool.
func (*routableBalancerPoolImpl) GetSQSType() domain.SQSPoolType {
	return domain.Balancer
}

// GetCodeID implements domain.RoutablePool.
func (r *routableBalancerPoolImpl) GetCodeID() uint64 {
	return notCosmWasmPoolCodeID
}
