package route

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

var _ domain.Route = &RouteImpl{}

type RouteImpl struct {
	Pools []domain.RoutablePool `json:"pools"`
	// HasGeneralizedCosmWasmPool is true if the route contains a generalized cosmwasm pool.
	// We track whether a route contains a generalized cosmwasm pool
	// so that we can exclude it from split quote logic.
	// The reason for this is that making network requests to chain is expensive.
	// As a result, we want to minimize the number of requests we make.
	HasGeneralizedCosmWasmPool bool `json:"has-cw-pool"`
	// HasCanonicalOrderbookPool is true if the route contains a canonical orderbook pool.
	HasCanonicalOrderbookPool bool `json:"-"`
}

type RouteImpls []RouteImpl

type RouteWithOutAmount struct {
	RouteImpl
	OutAmount osmomath.Int `json:"out_amount"`
	InAmount  osmomath.Int `json:"in_amount"`
}

var _ domain.SplitRoute = &RouteWithOutAmount{}

// GetAmountIn implements domain.SplitRoute.
func (r RouteWithOutAmount) GetAmountIn() osmomath.Int {
	return r.InAmount
}

// GetAmountOut implements domain.SplitRoute.
func (r RouteWithOutAmount) GetAmountOut() osmomath.Int {
	return r.OutAmount
}

// CalculateTokenOutByTokenIn calculates the token out amount given the token in amount for each route in r.
// Returns slice errors for each route than failed to calculate token out.
func (r RouteImpls) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) ([]RouteWithOutAmount, []error) {
	type result struct {
		index int
		data  RouteWithOutAmount
		err   error
	}

	routesWithAmountOut := make([]RouteWithOutAmount, 0, len(r))
	results := make(chan result, len(r))

	// spin up goroutines to calculate token out for each route
	var wg sync.WaitGroup
	for i, route := range r {
		wg.Add(1)
		go func(i int, r RouteImpl) {
			defer wg.Done()

			directRouteTokenOut, err := r.CalculateTokenOutByTokenIn(ctx, tokenIn)
			if err != nil {
				results <- result{index: i, err: err}
				return
			}

			if directRouteTokenOut.Amount.IsNil() {
				directRouteTokenOut.Amount = osmomath.ZeroInt()
			}

			results <- result{
				index: i,
				data: RouteWithOutAmount{
					RouteImpl: r,
					InAmount:  tokenIn.Amount,
					OutAmount: directRouteTokenOut.Amount,
				},
			}
		}(i, route)
	}

	wg.Wait()      // wait for all goroutines to finish
	close(results) // close the channel so we can range over it

	// collect results
	var errors []error
	var idx []int
	for range len(r) {
		res := <-results
		if res.err != nil {
			errors = append(errors, res.err)
			continue
		}
		idx = append(idx, res.index)
		routesWithAmountOut = append(routesWithAmountOut, res.data)
	}

	// Sort routes by index to maintain original order
	for i := 0; i < len(idx); i++ {
		for j := i + 1; j < len(idx); j++ {
			if idx[i] > idx[j] {
				routesWithAmountOut[i], routesWithAmountOut[j] = routesWithAmountOut[j], routesWithAmountOut[i]
			}
		}
	}

	return routesWithAmountOut, errors
}

var spotPriceErrorResultCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "sqs_routes_spot_price_error_total",
		Help: "Spot price error when preparing result pools",
	},
	[]string{"token_in", "cur_token_out_denom", "route_token_out_denom"},
)

// PrepareResultPoolsOutGivenIn implements domain.Route.
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
func (r RouteImpl) PrepareResultPoolsOutGivenIn(ctx context.Context, tokenIn sdk.Coin, spotPriceCalculator domain.SpotPriceQuoteCalculator, logger log.Logger) ([]domain.RoutablePool, osmomath.Dec, osmomath.Dec, error) {
	var (
		routeSpotPriceInBaseOutQuote     = osmomath.OneDec()
		effectiveSpotPriceInBaseOutQuote = osmomath.OneDec()
	)

	newPools := make([]domain.RoutablePool, 0, len(r.Pools))

	for _, pool := range r.Pools {
		// Compute spot price before swap.
		var (
			spotPriceInBaseOutQuote osmomath.BigDec
			err                     error
		)
		if pool.GetSQSType() == domain.Orderbook {
			spotPriceInBaseOutQuote, err = spotPriceCalculator.CalcSpotPrice(ctx, tokenIn.Denom, pool.GetTokenOutDenom())
			// spotPriceInBaseOutQuote, err = pool.CalcSpotPrice(ctx, tokenIn.Denom, pool.GetTokenOutDenom())
		} else {
			spotPriceInBaseOutQuote, err = pool.CalcSpotPrice(ctx, tokenIn.Denom, pool.GetTokenOutDenom())
		}
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

		newPool.TokenIn = tokenIn
		newPool.TokenOut = tokenOut
		newPool.SpotPrice = spotPriceInBaseOutQuote
		newPool.EffectiveSpotPrice = tokenOut.Amount.ToLegacyDec().QuoMut(tokenIn.Amount.ToLegacyDec())

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

// CalculateTokenInByTokenOut implements Route.
func (r *RouteImpl) CalculateTokenInByTokenOut(ctx context.Context, tokenOut sdk.Coin) (tokenIn sdk.Coin, err error) {
	defer func() {
		// TODO: cover this by test
		if r := recover(); r != nil {
			tokenIn = sdk.Coin{}
			err = fmt.Errorf("error when calculating in by out in route: %v", r)
		}
	}()

	for _, pool := range r.Pools {
		tokenIn, err = pool.CalculateTokenInByTokenOut(ctx, tokenOut)
		if err != nil {
			return sdk.Coin{}, err
		}

		// Charge taker fee
		tokenIn = pool.ChargeTakerFeeExactOut(tokenIn)

		tokenInAmt := tokenIn.Amount.ToLegacyDec()

		if tokenInAmt.IsNil() || tokenInAmt.IsZero() {
			return sdk.Coin{}, nil
		}

		tokenOut = tokenIn
	}

	return tokenIn, nil
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
