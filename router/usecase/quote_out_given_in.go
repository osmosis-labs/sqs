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
	one = osmomath.OneDec()
)

var (
	_ domain.Quote = &quoteExactAmountIn{}
)

// NOTE: This is because structs in alias declaration are not exported
type (
	QuoteExactAmountOut = quoteExactAmountOut
	QuoteExactAmountIn  = quoteExactAmountIn
)

func NewQuoteExactAmountOut(q *QuoteExactAmountIn) *quoteExactAmountOut {
	return &quoteExactAmountOut{
		quoteExactAmountIn: q,
	}
}

// quoteExactAmountIn is a quote implementation for token swap method exact in.
type quoteExactAmountIn struct {
	AmountIn                sdk.Coin            "json:\"amount_in\""
	AmountOut               osmomath.Int        "json:\"amount_out\""
	Route                   []domain.SplitRoute "json:\"route\""
	EffectiveFee            osmomath.Dec        "json:\"effective_fee\""
	PriceImpact             osmomath.Dec        "json:\"price_impact\""
	InBaseOutQuoteSpotPrice osmomath.Dec        "json:\"in_base_out_quote_spot_price\""
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
func (q *quoteExactAmountIn) PrepareResult(ctx context.Context, scalingFactor osmomath.Dec, logger log.Logger) ([]domain.SplitRoute, osmomath.Dec, error) {
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
			poolTakerFee := pool.GetTakerFee()

			routeTotalFee.AddMut(
				//  (1 - routeTotalFee) * poolTakerFee
				osmomath.OneDec().SubMut(routeTotalFee).MulTruncateMut(poolTakerFee),
			)
		}

		// Update the spread factor pro-rated by the amount in
		totalFeeAcrossRoutes.AddMut(routeTotalFee.MulMut(routeAmountInFraction))

		amountInFraction := q.AmountIn.Amount.ToLegacyDec().MulMut(routeAmountInFraction).TruncateInt()
		newPools, routeSpotPriceInBaseOutQuote, effectiveSpotPriceInBaseOutQuote, err := curRoute.PrepareResultPools(ctx, sdk.NewCoin(q.AmountIn.Denom, amountInFraction), logger)
		if err != nil {
			return nil, osmomath.Dec{}, err
		}

		totalSpotPriceInBaseOutQuote = totalSpotPriceInBaseOutQuote.AddMut(routeSpotPriceInBaseOutQuote.MulMut(routeAmountInFraction))
		totalEffectiveSpotPriceInBaseOutQuote = totalEffectiveSpotPriceInBaseOutQuote.AddMut(effectiveSpotPriceInBaseOutQuote.MulMut(routeAmountInFraction))

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
	if !totalSpotPriceInBaseOutQuote.IsZero() {
		q.PriceImpact = totalEffectiveSpotPriceInBaseOutQuote.Quo(totalSpotPriceInBaseOutQuote).SubMut(one)
	}

	q.EffectiveFee = totalFeeAcrossRoutes
	q.Route = resultRoutes
	q.InBaseOutQuoteSpotPrice = totalSpotPriceInBaseOutQuote

	return q.Route, q.EffectiveFee, nil
}

// GetAmountIn implements Quote.
func (q *quoteExactAmountIn) GetAmountIn() sdk.Coin {
	return q.AmountIn
}

// GetAmountOut implements Quote.
func (q *quoteExactAmountIn) GetAmountOut() osmomath.Int {
	return q.AmountOut
}

// GetRoute implements Quote.
func (q *quoteExactAmountIn) GetRoute() []domain.SplitRoute {
	return q.Route
}

// GetEffectiveFee implements Quote.
func (q *quoteExactAmountIn) GetEffectiveFee() osmomath.Dec {
	return q.EffectiveFee
}

// String implements domain.Quote.
func (q *quoteExactAmountIn) String() string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Quote: %s in for %s out \n", q.AmountIn, q.AmountOut))

	for _, route := range q.Route {
		builder.WriteString(route.String())
	}

	return builder.String()
}

// GetPriceImpact implements domain.Quote.
func (q *quoteExactAmountIn) GetPriceImpact() osmomath.Dec {
	return q.PriceImpact
}

// GetInBaseOutQuoteSpotPrice implements domain.Quote.
func (q *quoteExactAmountIn) GetInBaseOutQuoteSpotPrice() osmomath.Dec {
	return q.InBaseOutQuoteSpotPrice
}

// SetQuotePriceInfo implements domain.Quote.
func (q *quoteExactAmountIn) SetQuotePriceInfo(info *domain.TxFeeInfo) {
	q.PriceInfo = info
}
