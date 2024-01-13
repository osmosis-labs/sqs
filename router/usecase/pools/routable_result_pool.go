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
	"github.com/osmosis-labs/osmosis/v21/x/poolmanager"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v21/x/poolmanager/types"
)

var (
	_ sqsdomain.RoutablePool    = &routableResultPoolImpl{}
	_ domain.RoutableResultPool = &routableResultPoolImpl{}
)

// routableResultPoolImpl is a generalized implementation that is returned to the client
// side in quotes. It contains all the relevant pool data needed for Osmosis frontend
type routableResultPoolImpl struct {
	ID            uint64                    "json:\"id\""
	Type          poolmanagertypes.PoolType "json:\"type\""
	Balances      sdk.Coins                 "json:\"balances\""
	SpreadFactor  osmomath.Dec              "json:\"spread_factor\""
	TokenOutDenom string                    "json:\"token_out_denom\""
	TakerFee      osmomath.Dec              "json:\"taker_fee\""
}

// NewRoutableResultPool returns the new routable result pool with the given parameters.
func NewRoutableResultPool(ID uint64, poolType poolmanagertypes.PoolType, spreadFactor osmomath.Dec, tokenOutDenom string, takerFee osmomath.Dec) sqsdomain.RoutablePool {
	return &routableResultPoolImpl{
		ID:            ID,
		Type:          poolType,
		SpreadFactor:  spreadFactor,
		TokenOutDenom: tokenOutDenom,
		TakerFee:      takerFee,
	}
}

// GetId implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetId() uint64 {
	return r.ID
}

// GetPoolDenoms implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetPoolDenoms() []string {
	denoms := make([]string, len(r.Balances))
	for i, balance := range r.Balances {
		denoms[i] = balance.Denom
	}

	return denoms
}

// GetSQSPoolModel implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetSQSPoolModel() sqsdomain.SQSPool {
	return sqsdomain.SQSPool{
		Balances:     r.Balances,
		PoolDenoms:   r.GetPoolDenoms(),
		SpreadFactor: r.SpreadFactor,
	}
}

// GetTickModel implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetTickModel() (*sqsdomain.TickModel, error) {
	return nil, errors.New("not implemented")
}

// GetTotalValueLockedUOSMO implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetTotalValueLockedUOSMO() math.Int {
	return osmomath.Int{}
}

// GetType implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetType() poolmanagertypes.PoolType {
	return r.Type
}

// GetUnderlyingPool implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetUnderlyingPool() poolmanagertypes.PoolI {
	return nil
}

// Validate implements sqsdomain.RoutablePool.
func (*routableResultPoolImpl) Validate(minUOSMOTVL math.Int) error {
	return nil
}

// CalculateTokenOutByTokenIn implements RoutablePool.
func (r *routableResultPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	return sdk.Coin{}, errors.New("not implemented")
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableResultPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// String implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d), pool denoms (%v)", r.GetId(), r.GetType(), r.GetPoolDenoms())
}

// ChargeTakerFee implements sqsdomain.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (r *routableResultPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.TakerFee)
	return tokenInAfterTakerFee
}

// GetTakerFee implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// GetBalances implements domain.RoutableResultPool.
func (r *routableResultPoolImpl) GetBalances() sdk.Coins {
	return r.Balances
}

// SetTokenOutDenom implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// GetSpreadFactor implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

// CalcSpotPrice implements sqsdomain.RoutablePool.
func (r *routableResultPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	panic("not implemented")
}

// IsGeneralizedCosmWasmPool implements sqsdomain.RoutablePool.
func (*routableResultPoolImpl) IsGeneralizedCosmWasmPool() bool {
	return false
}
