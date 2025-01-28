package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/types"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ domain.Quote = &quoteExactAmountOut{}
)

// quoteExactAmountOut is a quote wrapper for exact out quotes.
// Note that only the PrepareResult method is different from the quoteExactAmountIn.
type quoteExactAmountOut struct {
	*quoteExactAmountIn     "json:\"-\""
	AmountIn                osmomath.Int        `json:"amount_in"`
	AmountOut               sdk.Coin            `json:"amount_out"`
	Route                   []domain.SplitRoute `json:"route"`
	EffectiveFee            osmomath.Dec        `json:"effective_fee"`
	PriceImpact             osmomath.Dec        `json:"price_impact"`
	InBaseOutQuoteSpotPrice osmomath.Dec        `json:"in_base_out_quote_spot_price"`
	PriceInfo               *domain.TxFeeInfo   `json:"price_info,omitempty"`
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

// PrepareResult implements domain.Quote.
// PrepareResult mutates the quote to prepare
// it with the data formatted for output to the client.
// Specifically:
// It strips away unnecessary fields from each pool in the route.
// Computes an effective spread factor from all routes.
//
// Returns the updated route and the effective spread factor.
func (q *quoteExactAmountOut) PrepareResult(ctx context.Context, scalingFactor osmomath.Dec, logger log.Logger) ([]domain.SplitRoute, osmomath.Dec, error) {
	if q.quoteExactAmountIn != nil {
		// Prepare exact out in the quote for inputs inversion
		if _, _, err := q.quoteExactAmountIn.PrepareResult(ctx, scalingFactor, logger); err != nil {
			return nil, osmomath.Dec{}, err
		}

		// Assign the inverted values to the quote
		q.AmountOut = q.quoteExactAmountIn.AmountIn
		q.AmountIn = q.quoteExactAmountIn.AmountOut
		q.Route = q.quoteExactAmountIn.Route
		q.EffectiveFee = q.quoteExactAmountIn.EffectiveFee
		q.PriceImpact = q.quoteExactAmountIn.PriceImpact
		q.InBaseOutQuoteSpotPrice = q.quoteExactAmountIn.InBaseOutQuoteSpotPrice

		for i, route := range q.Route {
			route, ok := route.(*RouteWithOutAmount)
			if !ok {
				return nil, osmomath.Dec{}, types.ErrInvalidRouteType
			}

			// invert the in and out amounts
			route.InAmount, route.OutAmount = route.OutAmount, route.InAmount

			q.Route[i] = route

			// invert the in and out amounts for each pool
			for _, p := range route.GetPools() {
				p.SetTokenInDenom(p.GetTokenOutDenom())
				p.SetTokenOutDenom("")
			}
		}

		return q.Route, q.EffectiveFee, nil
	}

	totalAmountOut := q.AmountOut.Amount.ToLegacyDec()
	totalFeeAcrossRoutes := osmomath.ZeroDec()

	totalSpotPriceOutBaseInQuote := osmomath.ZeroDec()
	totalEffectiveSpotPriceOutBaseInQuote := osmomath.ZeroDec()

	resultRoutes := make([]domain.SplitRoute, 0, len(q.Route))

	for _, curRoute := range q.Route {
		routeTotalFee := osmomath.ZeroDec()
		routeAmountOutFraction := curRoute.GetAmountOut().ToLegacyDec().Quo(totalAmountOut)

		// Calculate the spread factor across pools in the route
		for _, pool := range curRoute.GetPools() {
			poolTakerFee := pool.GetTakerFee()

			routeTotalFee.AddMut(
				//  (1 - routeTotalFee) * poolTakerFee
				osmomath.OneDec().SubMut(routeTotalFee).MulTruncateMut(poolTakerFee),
			)
		}

		// Update the spread factor pro-rated by the amount in
		totalFeeAcrossRoutes.AddMut(routeTotalFee.MulMut(routeAmountOutFraction))

		amountOutFraction := q.AmountOut.Amount.ToLegacyDec().MulMut(routeAmountOutFraction).TruncateInt()
		newPools, routeSpotPriceOutBaseInQuote, effectiveSpotPriceOutBaseInQuote, err := curRoute.PrepareResultPoolsExactAmountOut(ctx, sdk.NewCoin(q.AmountOut.Denom, amountOutFraction), logger)
		if err != nil {
			return nil, osmomath.Dec{}, err
		}

		totalSpotPriceOutBaseInQuote = totalSpotPriceOutBaseInQuote.AddMut(routeSpotPriceOutBaseInQuote.MulMut(routeAmountOutFraction))
		totalEffectiveSpotPriceOutBaseInQuote = totalEffectiveSpotPriceOutBaseInQuote.AddMut(effectiveSpotPriceOutBaseInQuote.MulMut(routeAmountOutFraction))

		resultRoutes = append(resultRoutes, &RouteWithOutAmount{
			RouteImpl: route.RouteImpl{
				Pools:                      newPools,
				HasGeneralizedCosmWasmPool: curRoute.ContainsGeneralizedCosmWasmPool(),
			},
			InAmount:  curRoute.GetAmountIn(),
			OutAmount: curRoute.GetAmountOut(),
		})
	}

	// Calculate price impact
	if !totalSpotPriceOutBaseInQuote.IsZero() {
		q.PriceImpact = totalEffectiveSpotPriceOutBaseInQuote.Quo(totalSpotPriceOutBaseInQuote).SubMut(one)
	}

	q.EffectiveFee = totalFeeAcrossRoutes
	q.Route = resultRoutes
	q.InBaseOutQuoteSpotPrice = totalSpotPriceOutBaseInQuote

	return q.Route, q.EffectiveFee, nil
}
