package route

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/v22/osmomath"
)

var _ domain.Route = &RouteImpl{}

type RouteImpl struct {
	Pools []sqsdomain.RoutablePool "json:\"pools\""
	// HasGeneralizedCosmWasmPool is true if the route contains a generalized cosmwasm pool.
	// We track whether a route contains a generalized cosmwasm pool
	// so that we can exclude it from split quote logic.
	// The reason for this is that making network requests to chain is expensive.
	// As a result, we want to minimize the number of requests we make.
	HasGeneralizedCosmWasmPool bool "json:\"has-cw-pool\""
}

// PrepareResultPools implements domain.Route.
// Strips away unnecessary fields from each pool in the route,
// leaving only the data needed by client
// The following are the list of fields that are returned to the client in each pool:
// - ID
// - Type
// - Balances
// - Spread Factor
// - Token Out Denom
// - Taker Fee
// Note that it mutates the route.
// Returns spot price before swap and the effective spot price
func (r *RouteImpl) PrepareResultPools(ctx context.Context, tokenIn sdk.Coin) (osmomath.Dec, osmomath.Dec, error) {
	var (
		routeSpotPriceInOverOut     = osmomath.OneDec()
		effectiveSpotPriceInOverOut = osmomath.OneDec()
	)

	for i, pool := range r.Pools {
		// Compute spot price before swap.
		spotPriceInOverOut, err := pool.CalcSpotPrice(ctx, pool.GetTokenOutDenom(), tokenIn.Denom)
		if err != nil {
			return osmomath.Dec{}, osmomath.Dec{}, err
		}

		// Charge taker fee
		tokenIn = pool.ChargeTakerFeeExactIn(tokenIn)

		tokenOut, err := pool.CalculateTokenOutByTokenIn(ctx, tokenIn)
		if err != nil {
			return osmomath.Dec{}, osmomath.Dec{}, err
		}

		// Update effective spot price
		effectiveSpotPriceInOverOut.MulMut(tokenIn.Amount.ToLegacyDec().QuoMut(tokenOut.Amount.ToLegacyDec()))

		// Note, in the future we may want to increase the precision of the spot price
		routeSpotPriceInOverOut.MulMut(spotPriceInOverOut.Dec())

		r.Pools[i] = pools.NewRoutableResultPool(
			pool.GetId(),
			pool.GetType(),
			pool.GetSpreadFactor(),
			pool.GetTokenOutDenom(),
			pool.GetTakerFee(),
		)

		tokenIn = tokenOut
	}
	return routeSpotPriceInOverOut, effectiveSpotPriceInOverOut, nil
}

// GetPools implements Route.
func (r *RouteImpl) GetPools() []sqsdomain.RoutablePool {
	return r.Pools
}

// CalculateTokenOutByTokenIn implements Route.
func (r *RouteImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (tokenOut sdk.Coin, err error) {
	defer func() {
		// TODO: cover this by test
		if r := recover(); r != nil {
			tokenOut = sdk.Coin{}
			err = fmt.Errorf("error when calculating out by in in route: %v", r)
		}
	}()

	for _, pool := range r.Pools {
		// Charge taker fee
		tokenIn = pool.ChargeTakerFeeExactIn(tokenIn)
		tokenInAmt := tokenIn.Amount.ToLegacyDec()

		if tokenInAmt.IsNil() || tokenInAmt.IsZero() {
			return sdk.Coin{}, nil
		}

		tokenOut, err = pool.CalculateTokenOutByTokenIn(ctx, tokenIn)
		if err != nil {
			return sdk.Coin{}, err
		}

		tokenIn = tokenOut
	}

	return tokenOut, nil
}

// String implements domain.Route.
func (r *RouteImpl) String() string {
	var strBuilder strings.Builder
	for _, pool := range r.Pools {
		_, err := strBuilder.WriteString(fmt.Sprintf("{{%s %s}}", pool.String(), pool.GetTokenOutDenom()))
		if err != nil {
			panic(err)
		}
	}

	return strBuilder.String()
}

// GetTokenOutDenom implements domain.Route.
// Returns token out denom of the last pool in the route.
// If route is empty, returns empty string.
func (r *RouteImpl) GetTokenOutDenom() string {
	if len(r.Pools) == 0 {
		return ""
	}

	return r.Pools[len(r.Pools)-1].GetTokenOutDenom()
}

// ContainsGeneralizedCosmWasmPool implements domain.Route.
func (r *RouteImpl) ContainsGeneralizedCosmWasmPool() bool {
	return r.HasGeneralizedCosmWasmPool
}
