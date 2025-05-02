package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/types"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ domain.Quote = &quoteExactAmountOut{}

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
	// falling back to implementation based on inverting q.quoteExactAmountIn
	if q.quoteExactAmountIn != nil {
		return q.quoteExactAmountIn.GetAmountIn()
	}

	// in a new implementation q.quoteExactAmountIn is no longer set
	return sdk.Coin{Amount: q.AmountIn}
}

// GetAmountOut implements Quote.
func (q *quoteExactAmountOut) GetAmountOut() sdk.Coin {
	// falling back to implementation based on inverting q.quoteExactAmountIn
	if q.quoteExactAmountIn != nil {
		return q.quoteExactAmountIn.GetAmountOut()
	}

	// in a new implementation q.quoteExactAmountIn is no longer set
	return q.AmountOut
}

// GetRoute implements Quote.
func (q *quoteExactAmountOut) GetRoute() []domain.SplitRoute {
	// falling back to implementation based on inverting q.quoteExactAmountIn
	if q.quoteExactAmountIn != nil {
		return q.quoteExactAmountIn.GetRoute()
	}

	// in a new implementation q.quoteExactAmountIn is no longer set
	return q.Route
}

// GetEffectiveFee implements Quote.
func (q *quoteExactAmountOut) GetEffectiveFee() osmomath.Dec {
	// falling back to implementation based on inverting q.quoteExactAmountIn
	if q.quoteExactAmountIn != nil {
		return q.quoteExactAmountIn.GetEffectiveFee()
	}

	// in a new implementation q.quoteExactAmountIn is no longer set
	return q.EffectiveFee
}

// String implements domain.Quote.
func (q *quoteExactAmountOut) String() string {
	// falling back to implementation based on inverting q.quoteExactAmountIn
	if q.quoteExactAmountIn != nil {
		return q.quoteExactAmountIn.String()
	}

	// in a new implementation q.quoteExactAmountIn is no longer set
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Quote: %s in for %s out \n", q.AmountIn, q.AmountOut))

	for _, route := range q.Route {
		builder.WriteString(route.String())
	}

	return builder.String()
}

// GetPriceImpact implements domain.Quote.
func (q *quoteExactAmountOut) GetPriceImpact() osmomath.Dec {
	// falling back to implementation based on inverting q.quoteExactAmountIn
	if q.quoteExactAmountIn != nil {
		return q.quoteExactAmountIn.GetPriceImpact()
	}

	// in a new implementation q.quoteExactAmountIn is no longer set
	return q.PriceImpact
}

// GetInBaseOutQuoteSpotPrice implements domain.Quote.
func (q *quoteExactAmountOut) GetInBaseOutQuoteSpotPrice() osmomath.Dec {
	// falling back to implementation based on inverting q.quoteExactAmountIn
	if q.quoteExactAmountIn != nil {
		return q.quoteExactAmountIn.GetInBaseOutQuoteSpotPrice()
	}

	// in a new implementation q.quoteExactAmountIn is no longer set
	return q.InBaseOutQuoteSpotPrice
}

// SetQuotePriceInfo implements domain.Quote.
func (q *quoteExactAmountOut) SetQuotePriceInfo(info *domain.TxFeeInfo) {
	// falling back to implementation based on inverting q.quoteExactAmountIn
	if q.quoteExactAmountIn != nil {
		q.quoteExactAmountIn.SetQuotePriceInfo(info)
		return
	}

	// in a new implementation q.quoteExactAmountIn is no longer set
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
func (q *quoteExactAmountOut) PrepareResult(ctx context.Context, scalingFactor osmomath.Dec, spotPriceCalculator domain.SpotPriceQuoteCalculator, logger log.Logger) ([]domain.SplitRoute, osmomath.Dec, error) {
	// Prepare exact out in the quote for inputs inversion
	if _, _, err := q.quoteExactAmountIn.PrepareResult(ctx, scalingFactor, spotPriceCalculator, logger); err != nil {
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
