package route

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/osmomath"

	"go.uber.org/zap"
)

var _ domain.Route = &RouteImpl{}

type RouteImpl struct {
	Pools []domain.RoutablePool "json:\"pools\""
	// HasGeneralizedCosmWasmPool is true if the route contains a generalized cosmwasm pool.
	// We track whether a route contains a generalized cosmwasm pool
	// so that we can exclude it from split quote logic.
	// The reason for this is that making network requests to chain is expensive.
	// As a result, we want to minimize the number of requests we make.
	HasGeneralizedCosmWasmPool bool "json:\"has-cw-pool\""
	// HasCanonicalOrderbookPool is true if the route contains a canonical orderbook pool.
	HasCanonicalOrderbookPool bool "json:\"-\""
}

var (
	spotPriceErrorResultCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_routes_spot_price_error_total",
			Help: "Spot price error when preparing result pools",
		},
		[]string{"token_in", "cur_token_out_denom", "route_token_out_denom"},
	)
)

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
// with token in as base and token out as quote.
func (r RouteImpl) PrepareResultPools(ctx context.Context, tokenIn sdk.Coin, logger log.Logger) ([]domain.RoutablePool, osmomath.Dec, osmomath.Dec, error) {
	var (
		routeSpotPriceInBaseOutQuote     = osmomath.OneDec()
		effectiveSpotPriceInBaseOutQuote = osmomath.OneDec()
	)

	newPools := make([]domain.RoutablePool, 0, len(r.Pools))

	for _, pool := range r.Pools {
		// Compute spot price before swap.
		spotPriceInBaseOutQuote, err := pool.CalcSpotPrice(ctx, tokenIn.Denom, pool.GetTokenOutDenom())
		if err != nil {
			logger.Error("failed to calculate spot price for pool", zap.Error(err))

			// We don't want to fail the entire quote if one pool fails to calculate spot price.
			// This might cause miestimaions downsream but we a
			spotPriceInBaseOutQuote = osmomath.ZeroBigDec()

			// Increment the counter for the error
			spotPriceErrorResultCounter.WithLabelValues(
				tokenIn.Denom,
				pool.GetTokenOutDenom(),
				r.Pools[len(r.Pools)-1].GetTokenOutDenom(),
			).Inc()
		}

		// Charge taker fee
		tokenIn = pool.ChargeTakerFeeExactIn(tokenIn)

		tokenOut, err := pool.CalculateTokenOutByTokenIn(ctx, tokenIn)
		if err != nil {
			return nil, osmomath.Dec{}, osmomath.Dec{}, err
		}

		// Update effective spot price
		effectiveSpotPriceInBaseOutQuote.MulMut(tokenOut.Amount.ToLegacyDec().QuoMut(tokenIn.Amount.ToLegacyDec()))

		// Note, in the future we may want to increase the precision of the spot price
		routeSpotPriceInBaseOutQuote.MulMut(spotPriceInBaseOutQuote.Dec())

		newPool := pools.NewRoutableResultPool(
			pool.GetId(),
			pool.GetType(),
			pool.GetSpreadFactor(),
			pool.GetTokenOutDenom(),
			pool.GetTakerFee(),
			pool.GetCodeID(),
		)

		newPools = append(newPools, newPool)

		tokenIn = tokenOut
	}
	return newPools, routeSpotPriceInBaseOutQuote, effectiveSpotPriceInBaseOutQuote, nil
}

// GetPools implements Route.
func (r *RouteImpl) GetPools() []domain.RoutablePool {
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

// GetTokenInDenom implements domain.Route.
// Returns token in denom of the last pool in the route.
// If route is empty, returns empty string.
func (r *RouteImpl) GetTokenInDenom() string {
	if len(r.Pools) == 0 {
		return ""
	}

	return r.Pools[len(r.Pools)-1].GetTokenInDenom()
}

// ContainsGeneralizedCosmWasmPool implements domain.Route.
func (r *RouteImpl) ContainsGeneralizedCosmWasmPool() bool {
	return r.HasGeneralizedCosmWasmPool
}

func (r *RouteImpl) UnmarshalJSON(data []byte) error {
	type Alias RouteImpl
	aux := &struct {
		Pools []json.RawMessage `json:"pools"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	r.Pools = make([]domain.RoutablePool, len(aux.Pools))
	for i, poolData := range aux.Pools {
		var resultPool pools.RoutableResultPoolImpl

		err := json.Unmarshal(poolData, &resultPool)
		if err != nil {
			return err
		}

		r.Pools[i] = &resultPool
	}

	return nil
}
