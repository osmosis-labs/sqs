package usecase

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/types"

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
	AmountIn                osmomath.Int        "json:\"amount_in\""
	AmountOut               sdk.Coin            "json:\"amount_out\""
	Route                   []domain.SplitRoute "json:\"route\""
	EffectiveFee            osmomath.Dec        "json:\"effective_fee\""
	PriceImpact             osmomath.Dec        "json:\"price_impact\""
	InBaseOutQuoteSpotPrice osmomath.Dec        "json:\"in_base_out_quote_spot_price\""
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

func (q *quoteExactAmountOut) GetRoute() []domain.SplitRoute {
	return q.Route
}

func (q *quoteExactAmountOut) UnmarshalJSON(data []byte) error {
	type Alias quoteExactAmountOut
	aux := &struct {
		Route json.RawMessage `json:"route"`
		*Alias
	}{
		Alias: (*Alias)(q),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse Route
	var routes []json.RawMessage
	if err := json.Unmarshal(aux.Route, &routes); err != nil {
		return fmt.Errorf("failed to parse Route: %w", err)
	}

	q.Route = make([]domain.SplitRoute, len(routes))
	for i, routeData := range routes {
		// Parse RouteImpl
		var routeWithAmounts RouteWithOutAmount
		if err := json.Unmarshal(routeData, &routeWithAmounts); err != nil {
			return fmt.Errorf("failed to parse routeWithAmounts: %w", err)
		}

		q.Route[i] = &routeWithAmounts
	}

	return nil
}
