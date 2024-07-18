package usecase

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"
)

type QuoteExactAmountOut struct {
	AmountIn                osmomath.Int        "json:\"amount_in\""
	AmountOut               sdk.Coin            "json:\"amount_out\""
	Route                   []domain.SplitRoute "json:\"route\""
	EffectiveFee            osmomath.Dec        "json:\"effective_fee\""
	PriceImpact             osmomath.Dec        "json:\"price_impact\""
	InBaseOutQuoteSpotPrice osmomath.Dec        "json:\"in_base_out_quote_spot_price\""
}

type quoteImpl struct {
	AmountIn                sdk.Coin            "json:\"amount_in\""
	AmountOut               osmomath.Int        "json:\"amount_out\""
	Route                   []domain.SplitRoute "json:\"route\""
	EffectiveFee            osmomath.Dec        "json:\"effective_fee\""
	PriceImpact             osmomath.Dec        "json:\"price_impact\""
	InBaseOutQuoteSpotPrice osmomath.Dec        "json:\"in_base_out_quote_spot_price\""
}

var (
	one = osmomath.OneDec()
)

var _ domain.Quote = &quoteImpl{}

// PrepareResult implements domain.Quote.
// PrepareResult mutates the quote to prepare
// it with the data formatted for output to the client.
// Specifically:
// It strips away unnecessary fields from each pool in the route.
// Computes an effective spread factor from all routes.
//
// Returns the updated route and the effective spread factor.
func (q *quoteImpl) PrepareResult(ctx context.Context, scalingFactor osmomath.Dec, method domain.TokenSwapMethod) ([]domain.SplitRoute, osmomath.Dec, error) {
	totalAmountIn := q.AmountIn.Amount.ToLegacyDec()
	totalFeeAcrossRoutes := osmomath.ZeroDec()

	totalSpotPriceInBaseOutQuote := osmomath.ZeroDec()
	totalEffectiveSpotPriceInBaseOutQuote := osmomath.ZeroDec()

	resultRoutes := make([]domain.SplitRoute, 0, len(q.Route))

	for _, curRoute := range q.Route {
		routeTotalFee := osmomath.ZeroDec()
		routeAmountInFraction := curRoute.GetAmountIn().ToLegacyDec().Quo(totalAmountIn)

		// Calculate the spread factor across pools in the route
		for _, pool := range curRoute.GetPools() {
			poolSpreadFactor := pool.GetSpreadFactor()
			poolTakerFee := pool.GetTakerFee()

			totalPoolFee := poolSpreadFactor.Add(poolTakerFee)

			routeTotalFee.AddMut(
				//  (1 - routeSpreadFactor) * poolSpreadFactor
				osmomath.OneDec().SubMut(routeTotalFee).MulTruncateMut(totalPoolFee),
			)
		}

		// Update the spread factor pro-rated by the amount in
		totalFeeAcrossRoutes.AddMut(routeTotalFee.MulMut(routeAmountInFraction))

		amountInFraction := q.AmountIn.Amount.ToLegacyDec().MulMut(routeAmountInFraction).TruncateInt()
		newPools, routeSpotPriceInBaseOutQuote, effectiveSpotPriceInBaseOutQuote, err := curRoute.PrepareResultPools(ctx, sdk.NewCoin(q.AmountIn.Denom, amountInFraction), domain.TokenSwapMethodExactIn)
		if err != nil {
			return nil, osmomath.Dec{}, err
		}

		totalSpotPriceInBaseOutQuote = totalSpotPriceInBaseOutQuote.AddMut(routeSpotPriceInBaseOutQuote.MulMut(routeAmountInFraction))
		totalEffectiveSpotPriceInBaseOutQuote = totalEffectiveSpotPriceInBaseOutQuote.AddMut(effectiveSpotPriceInBaseOutQuote.MulMut(routeAmountInFraction))

		route := RouteWithOutAmount{
			RouteImpl: route.RouteImpl{
				Pools:                      newPools,
				HasGeneralizedCosmWasmPool: curRoute.ContainsGeneralizedCosmWasmPool(),
			},
			InAmount:  curRoute.GetAmountIn(),
			OutAmount: curRoute.GetAmountOut(),
		}

		// invert the in and out amounts if the method is exact out
		if method == domain.TokenSwapMethodExactOut {
			route.InAmount, route.OutAmount = route.OutAmount, route.InAmount
		}

		resultRoutes = append(resultRoutes, &route)
	}

	// Calculate price impact
	if !totalSpotPriceInBaseOutQuote.IsZero() {
		q.PriceImpact = totalEffectiveSpotPriceInBaseOutQuote.Quo(totalSpotPriceInBaseOutQuote).SubMut(one)
	}

	q.EffectiveFee = totalFeeAcrossRoutes
	q.Route = resultRoutes
	q.InBaseOutQuoteSpotPrice = totalSpotPriceInBaseOutQuote

	return q.Route, q.EffectiveFee, nil
}

func (q *QuoteExactAmountOut) PrepareResult(ctx context.Context, scalingFactor osmomath.Dec) ([]domain.SplitRoute, osmomath.Dec, error) {
	for _, r := range q.Route {
		for _, p := range r.GetPools() {
			p.SetTokenInDenom(p.GetTokenOutDenom())
			p.SetTokenOutDenom("")
		}
	}

	return q.Route, q.EffectiveFee, nil
}

// GetAmountIn implements Quote.
func (q *quoteImpl) GetAmountIn() sdk.Coin {
	return q.AmountIn
}

// GetAmountOut implements Quote.
func (q *quoteImpl) GetAmountOut() osmomath.Int {
	return q.AmountOut
}

// GetRoute implements Quote.
func (q *quoteImpl) GetRoute() []domain.SplitRoute {
	return q.Route
}

// GetEffectiveSpreadFactor implements Quote.
func (q *quoteImpl) GetEffectiveSpreadFactor() osmomath.Dec {
	return q.EffectiveFee
}

// String implements domain.Quote.
func (q *quoteImpl) String() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Quote: %s in for %s out \n", q.AmountIn, q.AmountOut))

	for _, route := range q.Route {
		builder.WriteString(route.String())
	}

	return builder.String()
}

// GetPriceImpact implements domain.Quote.
func (q *quoteImpl) GetPriceImpact() osmomath.Dec {
	return q.PriceImpact
}

// GetInBaseOutQuoteSpotPrice implements domain.Quote.
func (q *quoteImpl) GetInBaseOutQuoteSpotPrice() osmomath.Dec {
	return q.InBaseOutQuoteSpotPrice
}
