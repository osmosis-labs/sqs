package pools

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	clmath "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/math"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

var _ sqsdomain.RoutablePool = &routableOrderbookPoolImpl{}

type routableOrderbookPoolImpl struct {
	ChainPool     *cwpoolmodel.CosmWasmPool     "json:\"pool\""
	Balances      sdk.Coins                     "json:\"balances\""
	TokenOutDenom string                        "json:\"token_out_denom\""
	TakerFee      osmomath.Dec                  "json:\"taker_fee\""
	SpreadFactor  osmomath.Dec                  "json:\"spread_factor\""
	TickModel     *sqsdomain.OrderbookTickModel "json:\"orderbook_tick_model\""
	QuoteDenom    string                        "json:\"quote_denom\""
	BaseDenom     string                        "json:\"base_denom\""
}

// GetId implements sqsdomain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetId() uint64 {
	return r.ChainPool.PoolId
}

// GetPoolDenoms implements sqsdomain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetPoolDenoms() []string {
	return r.Balances.Denoms()
}

// GetType implements sqsdomain.RoutablePool.
func (*routableOrderbookPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.CosmWasm
}

// GetSpreadFactor implements sqsdomain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

func (r *routableOrderbookPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	poolType := r.GetType()

	// Esnure that the pool is a cosmwasm pool
	if poolType != poolmanagertypes.CosmWasm {
		return sdk.Coin{}, domain.InvalidPoolTypeError{PoolType: int32(poolType)}
	}

	// Get the expected order direction
	direction, err := r.GetDirection(tokenIn.Denom, r.TokenOutDenom)
	if err != nil {
		return sdk.Coin{}, err
	}

	tickIdx, err := r.GetStartTickIndex(direction)
	if err != nil || tickIdx == -1 {
		return sdk.Coin{}, err
	}

	amountOutTotal := osmomath.ZeroBigDec()
	amountInRemaining := osmomath.BigDecFromSDKInt(tokenIn.Amount)

	// ASSUMPTION: Ticks are ordered
	for amountInRemaining.GT(zeroBigDec) {
		if tickIdx >= len(r.TickModel.TickStates) || tickIdx < 0 {
			return sdk.Coin{}, sqsdomain.OrderbookNotEnoughLiquidityToCompleteSwapError{PoolId: r.GetId(), AmountIn: tokenIn}
		}
		tick := r.TickModel.TickStates[tickIdx]

		if direction == sqsdomain.ASK {
			tickIdx++
		} else if direction == sqsdomain.BID {
			tickIdx--
		} else {
			return sdk.Coin{}, sqsdomain.OrderbookPoolInvalidDirectionError{Direction: direction}
		}

		// Calculate the price for the current tick
		tickPrice, err := clmath.TickToPrice(tick.TickId)
		if err != nil {
			return sdk.Coin{}, err
		}

		// How much of the other denom is being filled by this order
		outputAmount := amountToValue(osmomath.BigDecFromSDKInt(tokenIn.Amount), tickPrice, direction)

		// The current state for the tick given the current direction
		tickValues, err := tick.GetTickValues(direction)
		if err != nil {
			return sdk.Coin{}, err
		}

		// How much of the order this tick can fill given the current direction
		fillAmount := tickValues.GetFillableAmount(outputAmount)

		// How much of the original denom has been filled
		inputFilled := amountToValue(fillAmount, tickPrice, direction*-1)

		// Add the filled amount to the order total
		amountOutTotal = amountOutTotal.AddMut(inputFilled)

		// Subtract the filled amount from the remaining amount of tokens in
		amountInRemaining = amountInRemaining.SubMut(fillAmount)
	}

	// Return total amount out
	return sdk.Coin{r.TokenOutDenom, amountOutTotal.Dec().TruncateInt()}, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableOrderbookPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// String implements sqsdomain.RoutablePool.
func (r *routableOrderbookPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d) Orderbook, pool denoms (%v), token out (%s)", r.ChainPool.PoolId, poolmanagertypes.CosmWasm, r.GetPoolDenoms(), r.TokenOutDenom)
}

// ChargeTakerFeeExactIn implements sqsdomain.RoutablePool.
// Returns tokenInAmount and does not charge any fee for transmuter pools.
func (r *routableOrderbookPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (inAmountAfterFee sdk.Coin) {
	return tokenIn
}

// GetTakerFee implements sqsdomain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// SetTokenOutDenom implements sqsdomain.RoutablePool.
func (r *routableOrderbookPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// CalcSpotPrice implements sqsdomain.RoutablePool.
func (r *routableOrderbookPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	return osmomath.OneBigDec(), nil
}

// IsGeneralizedCosmWasmPool implements sqsdomain.RoutablePool.
func (*routableOrderbookPoolImpl) IsGeneralizedCosmWasmPool() bool {
	return false
}

// GetCodeID implements sqsdomain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetCodeID() uint64 {
	return r.ChainPool.CodeId
}

func (r *routableOrderbookPoolImpl) GetDirection(tokenInDenom, tokenOutDenom string) (int64, error) {
	if tokenInDenom == r.BaseDenom && tokenOutDenom == r.QuoteDenom {
		return sqsdomain.ASK, nil
	} else if tokenInDenom == r.QuoteDenom && tokenOutDenom == r.BaseDenom {
		return sqsdomain.BID, nil
	} else {
		return 0, sqsdomain.OrderbookPoolMismatchError{PoolId: r.GetId(), TokenInDenom: tokenInDenom, TokenOutDenom: tokenOutDenom}
	}
}

func (r *routableOrderbookPoolImpl) GetStartTickIndex(direction int64) (int, error) {
	if direction == sqsdomain.ASK {
		return r.TickModel.GetTickIndexById(r.TickModel.NextAskTickId), nil
	} else if direction == sqsdomain.BID {
		return r.TickModel.GetTickIndexById(r.TickModel.NextBidTickId), nil
	} else {
		return -1, sqsdomain.OrderbookPoolInvalidDirectionError{Direction: direction}
	}
}

func amountToValue(amount osmomath.BigDec, price osmomath.BigDec, direction int64) osmomath.BigDec {
	if direction == sqsdomain.ASK {
		return amount.Mul(price)
	} else {
		return amount.Quo(price)
	}
}
