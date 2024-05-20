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
	ChainPool     *cwpoolmodel.CosmWasmPool  "json:\"pool\""
	Balances      sdk.Coins                  "json:\"balances\""
	TokenOutDenom string                     "json:\"token_out_denom\""
	TakerFee      osmomath.Dec               "json:\"taker_fee\""
	SpreadFactor  osmomath.Dec               "json:\"spread_factor\""
	TickModel     *domain.OrderbookTickModel "json:\"orderbook_tick_model\""
	QuoteDenom    string                     "json:\"quote_denom\""
	BaseDenom     string                     "json:\"base_denom\""
}

// GetId implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetId() uint64 {
	return r.ChainPool.PoolId
}

// GetPoolDenoms implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetPoolDenoms() []string {
	return r.Balances.Denoms()
}

// GetType implements domain.RoutablePool.
func (*routableOrderbookPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.CosmWasm
}

// GetSpreadFactor implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

// CalculateTokenOutByTokenIn implements sqsdomain.RoutablePool.
// It calculates the amount of token out given the amount of token in for a concentrated liquidity pool.
// Fails if:
// - the underlying chain pool set on the routable pool is not of cosmwasm type
// - fails to retrieve the tick model for the pool
// - the provided denom pair is not supported by the orderbook
// - runs out of ticks during swap (token in is too high for liquidity in the pool)
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
		// Order has run out of ticks to iterate
		if tickIdx >= len(r.TickModel.TickStates) || tickIdx < 0 {
			return sdk.Coin{}, domain.OrderbookNotEnoughLiquidityToCompleteSwapError{PoolId: r.GetId(), AmountIn: tokenIn}
		}
		tick := r.TickModel.TickStates[tickIdx]

		// Increment or decrement the current tick index depending on order direction
		if direction == domain.ASK {
			tickIdx++
		} else if direction == domain.BID {
			tickIdx--
		} else {
			return sdk.Coin{}, domain.OrderbookPoolInvalidDirectionError{Direction: direction}
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

// String implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d) Orderbook, pool denoms (%v), token out (%s)", r.ChainPool.PoolId, poolmanagertypes.CosmWasm, r.GetPoolDenoms(), r.TokenOutDenom)
}

// ChargeTakerFeeExactIn implements domain.RoutablePool.
// Returns tokenInAmount and does not charge any fee for transmuter pools.
func (r *routableOrderbookPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (inAmountAfterFee sdk.Coin) {
	return tokenIn
}

// GetTakerFee implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// SetTokenOutDenom implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	return osmomath.OneBigDec(), nil
}

// IsGeneralizedCosmWasmPool implements domain.RoutablePool.
func (*routableOrderbookPoolImpl) IsGeneralizedCosmWasmPool() bool {
	return false
}

// GetCodeID implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetCodeID() uint64 {
	return r.ChainPool.CodeId
}

// Determines order direction for the current orderbook given token in and out denoms
// Returns:
// - 1 if the order is a bid (buying token out)
// - -1 if the order is an ask (selling token out)
// - 0 if the order is not valid
func (r *routableOrderbookPoolImpl) GetDirection(tokenInDenom, tokenOutDenom string) (int64, error) {
	if tokenInDenom == r.BaseDenom && tokenOutDenom == r.QuoteDenom {
		return domain.ASK, nil
	} else if tokenInDenom == r.QuoteDenom && tokenOutDenom == r.BaseDenom {
		return domain.BID, nil
	} else {
		return 0, domain.OrderbookPoolMismatchError{PoolId: r.GetId(), TokenInDenom: tokenInDenom, TokenOutDenom: tokenOutDenom}
	}
}

// Get the index for the tick state array for the starting index given direction
func (r *routableOrderbookPoolImpl) GetStartTickIndex(direction int64) (int, error) {
	if direction == domain.ASK {
		return r.TickModel.GetTickIndexById(r.TickModel.NextAskTickId), nil
	} else if direction == domain.BID {
		return r.TickModel.GetTickIndexById(r.TickModel.NextBidTickId), nil
	} else {
		return -1, domain.OrderbookPoolInvalidDirectionError{Direction: direction}
	}
}

// Converts an amount of token in to the value of token out given a price and direction
func amountToValue(amount osmomath.BigDec, price osmomath.BigDec, direction int64) osmomath.BigDec {
	if direction == domain.ASK {
		return amount.MulMut(price)
	} else {
		return amount.QuoMut(price)
	}
}
