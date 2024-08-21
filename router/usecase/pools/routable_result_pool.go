package pools

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v25/x/poolmanager"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

var (
	_ domain.RoutablePool       = &RoutableResultPoolImpl{}
	_ domain.RoutableResultPool = &RoutableResultPoolImpl{}
)

// RoutableResultPoolImpl is a generalized implementation that is returned to the client
// side in quotes. It contains all the relevant pool data needed for Osmosis frontend
type RoutableResultPoolImpl struct {
	ID            uint64                    "json:\"id\""
	Type          poolmanagertypes.PoolType "json:\"type\""
	Balances      sdk.Coins                 "json:\"balances\""
	SpreadFactor  osmomath.Dec              "json:\"spread_factor\""
	TokenOutDenom string                    "json:\"token_out_denom,omitempty\""
	TokenInDenom  string                    "json:\"token_in_denom,omitempty\""
	TakerFee      osmomath.Dec              "json:\"taker_fee\""
	CodeID        uint64                    "json:\"code_id,omitempty\""
}

// GetCodeID implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetCodeID() uint64 {
	panic("unimplemented")
}

// SetInDenom implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) SetInDenom(denom string) {
	r.TokenInDenom = denom
}

// SetOutDenom implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) SetOutDenom(denom string) {
	r.TokenOutDenom = denom
}

// NewRoutableResultPool returns the new routable result pool with the given parameters.
func NewRoutableResultPool(ID uint64, poolType poolmanagertypes.PoolType, spreadFactor osmomath.Dec, tokenOutDenom string, takerFee osmomath.Dec, codeID uint64) domain.RoutablePool {
	return &RoutableResultPoolImpl{
		ID:            ID,
		Type:          poolType,
		SpreadFactor:  spreadFactor,
		TokenOutDenom: tokenOutDenom,
		TakerFee:      takerFee,
		CodeID:        codeID,
	}
}

// NewExactAmountOutRoutableResultPool returns the new routable result pool with the given parameters.
func NewExactAmountOutRoutableResultPool(ID uint64, poolType poolmanagertypes.PoolType, spreadFactor osmomath.Dec, tokenInDenom string, takerFee osmomath.Dec, codeID uint64) domain.RoutablePool {
	return &RoutableResultPoolImpl{
		ID:           ID,
		Type:         poolType,
		SpreadFactor: spreadFactor,
		TokenInDenom: tokenInDenom,
		TakerFee:     takerFee,
		CodeID:       codeID,
	}
}

// GetId implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetId() uint64 {
	return r.ID
}

// GetPoolDenoms implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetPoolDenoms() []string {
	denoms := make([]string, len(r.Balances))
	for i, balance := range r.Balances {
		denoms[i] = balance.Denom
	}

	return denoms
}

// GetSQSPoolModel implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetSQSPoolModel() sqsdomain.SQSPool {
	return sqsdomain.SQSPool{
		Balances:     r.Balances,
		PoolDenoms:   r.GetPoolDenoms(),
		SpreadFactor: r.SpreadFactor,
	}
}

// GetTickModel implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetTickModel() (*sqsdomain.TickModel, error) {
	return nil, errors.New("not implemented")
}

// GetPoolLiquidityCap implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetPoolLiquidityCap() math.Int {
	return osmomath.Int{}
}

// GetType implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetType() poolmanagertypes.PoolType {
	return r.Type
}

// GetUnderlyingPool implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetUnderlyingPool() poolmanagertypes.PoolI {
	return nil
}

// Validate implements domain.RoutablePool.
func (*RoutableResultPoolImpl) Validate(minUOSMOTVL math.Int) error {
	return nil
}

// CalculateTokenOutByTokenIn implements RoutablePool.
func (r *RoutableResultPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	return sdk.Coin{}, errors.New("not implemented")
}

// GetTokenOutDenom implements RoutablePool.
func (r *RoutableResultPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (r *RoutableResultPoolImpl) GetTokenInDenom() string {
	return r.TokenInDenom
}

// String implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d), pool denoms (%v)", r.GetId(), r.GetType(), r.GetPoolDenoms())
}

// ChargeTakerFee implements domain.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (r *RoutableResultPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.TakerFee)
	return tokenInAfterTakerFee
}

// GetTakerFee implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// GetBalances implements domain.RoutableResultPool.
func (r *RoutableResultPoolImpl) GetBalances() sdk.Coins {
	return r.Balances
}

// SetTokenInDenom implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) SetTokenInDenom(tokenInDenom string) {
	r.TokenInDenom = tokenInDenom
}

// SetTokenOutDenom implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// GetSpreadFactor implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	panic("not implemented")
}

// GetSQSType implements domain.RoutablePool.
func (r *RoutableResultPoolImpl) GetSQSType() domain.SQSPoolType {
	return domain.Result
}
