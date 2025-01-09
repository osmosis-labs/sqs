package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ domain.Quote = &quoteExactAmountOut{}
)

// quoteExactAmountOut is a quote wrapper for exact out quotes.
// Note that only the PrepareResult method is different from the quoteExactAmountOut.
type quoteExactAmountOut struct {
	AmountIn                osmomath.Int        `json:"amount_in"`
	AmountOut               sdk.Coin            `json:"amount_out"`
	Route                   []domain.SplitRoute `json:"route"`
	EffectiveFee            osmomath.Dec        `json:"effective_fee"`
	PriceImpact             osmomath.Dec        `json:"price_impact"`
	InBaseOutQuoteSpotPrice osmomath.Dec        `json:"in_base_out_quote_spot_price"`
	PriceInfo               *domain.TxFeeInfo   `json:"price_info,omitempty"`
}

// PrepareResult implements domain.Quote.
// PrepareResult mutates the quote to prepare
// it with the data formatted for output to the client.
// Specifically:
// It strips away unnecessary fields from each pool in the route.
// Computes an effective spread factor from all routes.
//
// Returns the updated route and the effective spread factor.
func (q *quoteExactAmountOut) PrepareResult(ctx context.Context, scalingFactor osmomath.Dec, logger log.Logger) ([]domain.SplitRoute, osmomath.Dec, error) {
	totalAmountOut := q.AmountOut.Amount.ToLegacyDec()
	totalFeeAcrossRoutes := osmomath.ZeroDec()

	totalSpotPriceInBaseOutQuote := osmomath.ZeroDec()
	totalEffectiveSpotPriceInBaseOutQuote := osmomath.ZeroDec()

	resultRoutes := make([]domain.SplitRoute, 0, len(q.Route))

	for _, curRoute := range q.Route {
		routeTotalFee := osmomath.ZeroDec()
		routeAmountInFraction := curRoute.GetAmountIn().ToLegacyDec().Quo(totalAmountOut)

		// Calculate the spread factor across pools in the route
		for _, pool := range curRoute.GetPools() {
			poolTakerFee := pool.GetTakerFee()

			routeTotalFee.AddMut(
				//  (1 - routeTotalFee) * poolTakerFee
				osmomath.OneDec().SubMut(routeTotalFee).MulTruncateMut(poolTakerFee),
			)
		}

		// Update the spread factor pro-rated by the amount in
		totalFeeAcrossRoutes.AddMut(routeTotalFee.MulMut(routeAmountInFraction))

		amountInFraction := q.AmountOut.Amount.ToLegacyDec().MulMut(routeAmountInFraction).TruncateInt()
		newPools, routeSpotPriceInBaseOutQuote, effectiveSpotPriceInBaseOutQuote, err := curRoute.PrepareResultPoolsExactAmountOut(ctx, sdk.NewCoin(q.AmountOut.Denom, amountInFraction), logger)
		if err != nil {
			return nil, osmomath.Dec{}, err
		}

		totalSpotPriceInBaseOutQuote = totalSpotPriceInBaseOutQuote.AddMut(routeSpotPriceInBaseOutQuote.MulMut(routeAmountInFraction))
		totalEffectiveSpotPriceInBaseOutQuote = totalEffectiveSpotPriceInBaseOutQuote.AddMut(effectiveSpotPriceInBaseOutQuote.MulMut(routeAmountInFraction))

		resultRoutes = append(resultRoutes, &RouteWithAmount{
			RouteImpl: route.RouteImpl{
				Pools:                      newPools,
				HasGeneralizedCosmWasmPool: curRoute.ContainsGeneralizedCosmWasmPool(),
			},
			InAmount:  curRoute.GetAmountIn(),
			OutAmount: curRoute.GetAmountOut(),
		})
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

// GetAmountIn implements Quote.
func (q *quoteExactAmountOut) GetAmountIn() sdk.Coin {
	return sdk.Coin{Amount: q.AmountIn}
}

// GetAmountOut implements Quote.
func (q *quoteExactAmountOut) GetAmountOut() sdk.Coin {
	return q.AmountOut
}

// GetRoute implements Quote.
func (q *quoteExactAmountOut) GetRoute() []domain.SplitRoute {
	return q.Route
}

// GetEffectiveFee implements Quote.
func (q *quoteExactAmountOut) GetEffectiveFee() osmomath.Dec {
	return q.EffectiveFee
}

// String implements domain.Quote.
func (q *quoteExactAmountOut) String() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Quote: %s in for %s out \n", q.AmountIn, q.AmountOut))

	for _, route := range q.Route {
		builder.WriteString(route.String())
	}

	return builder.String()
}

// GetPriceImpact implements domain.Quote.
func (q *quoteExactAmountOut) GetPriceImpact() osmomath.Dec {
	return q.PriceImpact
}

// GetInBaseOutQuoteSpotPrice implements domain.Quote.
func (q *quoteExactAmountOut) GetInBaseOutQuoteSpotPrice() osmomath.Dec {
	return q.InBaseOutQuoteSpotPrice
}

// SetQuotePriceInfo implements domain.Quote.
func (q *quoteExactAmountOut) SetQuotePriceInfo(info *domain.TxFeeInfo) {
	q.PriceInfo = info
}
