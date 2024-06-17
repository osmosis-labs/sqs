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
	"github.com/osmosis-labs/osmosis/v25/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

var _ sqsdomain.RoutablePool = &routableOrderbookPoolImpl{}

type routableOrderbookPoolImpl struct {
	ChainPool     *cwpoolmodel.CosmWasmPool "json:\"pool\""
	Balances      sdk.Coins                 "json:\"balances\""
	TokenOutDenom string                    "json:\"token_out_denom\""
	TakerFee      osmomath.Dec              "json:\"taker_fee\""
	SpreadFactor  osmomath.Dec              "json:\"spread_factor\""
	OrderbookData *sqsdomain.OrderbookData  "json:\"orderbook_data\""
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
	if err != nil {
		return sdk.Coin{}, err
	}

	amountOutTotal := osmomath.ZeroBigDec()
	amountInRemaining := osmomath.BigDecFromSDKInt(tokenIn.Amount)

	// ASSUMPTION: Ticks are ordered
	for amountInRemaining.GT(zeroBigDec) {
		// Order has run out of ticks to iterate
		if tickIdx >= len(r.OrderbookData.Ticks) || tickIdx < 0 {
			return sdk.Coin{}, domain.OrderbookNotEnoughLiquidityToCompleteSwapError{PoolId: r.GetId(), AmountIn: tokenIn}
		}
		tick := r.OrderbookData.Ticks[tickIdx]

		// Increment or decrement the current tick index depending on order direction
		switch direction {
		case sqsdomain.ASK:
			tickIdx++
		case sqsdomain.BID:
			tickIdx--
		default:
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
		tickValues, err := tick.TickState.GetTickValues(direction)
		if err != nil {
			return sdk.Coin{}, err
		}

		// How much of the order this tick can fill given the current direction
		fillAmount := tickValues.GetFillableAmount(outputAmount)

		// How much of the original denom has been filled
		inputFilled := amountToValue(fillAmount, tickPrice, direction.Opposite())

		// Add the filled amount to the order total
		amountOutTotal = amountOutTotal.AddMut(inputFilled)

		// Subtract the filled amount from the remaining amount of tokens in
		amountInRemaining = amountInRemaining.SubMut(fillAmount)
	}

	// Return total amount out
	return sdk.Coin{Denom: r.TokenOutDenom, Amount: amountOutTotal.Dec().TruncateInt()}, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableOrderbookPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// String implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d) Orderbook, pool denoms (%v), token out (%s)", r.ChainPool.PoolId, poolmanagertypes.CosmWasm, r.GetPoolDenoms(), r.TokenOutDenom)
}

// ChargeTakerFee implements sqsdomain.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (r *routableOrderbookPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
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
	// Get the expected order direction
	direction, err := r.GetDirection(quoteDenom, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	tickIdx, err := r.GetStartTickIndex(direction)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	tick := r.OrderbookData.Ticks[tickIdx]

	// Calculate the price for the current tick
	return clmath.TickToPrice(tick.TickId)
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
func (r *routableOrderbookPoolImpl) GetDirection(tokenInDenom, tokenOutDenom string) (sqsdomain.OrderbookDirection, error) {
	if tokenInDenom == r.OrderbookData.BaseDenom && tokenOutDenom == r.OrderbookData.QuoteDenom {
		return sqsdomain.ASK, nil
	} else if tokenInDenom == r.OrderbookData.QuoteDenom && tokenOutDenom == r.OrderbookData.BaseDenom {
		return sqsdomain.BID, nil
	} else {
		return 0, domain.OrderbookPoolMismatchError{PoolId: r.GetId(), TokenInDenom: tokenInDenom, TokenOutDenom: tokenOutDenom}
	}
}

// Get the index for the tick state array for the starting index given direction
func (r *routableOrderbookPoolImpl) GetStartTickIndex(direction sqsdomain.OrderbookDirection) (int, error) {
	switch direction {
	case sqsdomain.ASK:
		return r.OrderbookData.GetTickIndexById(r.OrderbookData.NextAskTick), nil
	case sqsdomain.BID:
		return r.OrderbookData.GetTickIndexById(r.OrderbookData.NextBidTick), nil
	default:
		return -1, domain.OrderbookPoolInvalidDirectionError{Direction: direction}
	}
}

// Converts an amount of token in to the value of token out given a price and direction
func amountToValue(amount osmomath.BigDec, price osmomath.BigDec, direction sqsdomain.OrderbookDirection) osmomath.BigDec {
	switch direction {
	case sqsdomain.ASK:
		return amount.MulMut(price)
	case sqsdomain.BID:
		return amount.QuoMut(price)
	default:
		return osmomath.ZeroBigDec()
	}
}
