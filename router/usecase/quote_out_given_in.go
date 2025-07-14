package usecase

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var one = osmomath.OneDec()

var _ domain.Quote = &quoteExactAmountIn{}

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

// Token represents a token with its metadata.
type Token struct {
	Denom                   string       `json:"denom"`
	LiquidityCapitalization osmomath.Int `json:"liquidity_cap"`
}

// quoteExactAmountIn is a quote implementation for token swap method exact in.
type quoteExactAmountIn struct {
	AmountIn                sdk.Coin            `json:"amount_in"`
	AmountOut               osmomath.Int        `json:"amount_out"`
	Route                   []domain.SplitRoute `json:"route"`
	Tokens                  []Token             `json:"tokens"`
	LiquidityCap            osmomath.Int        `json:"liquidity_cap"`
	LiquidityCapOverflow    bool                `json:"liquidity_cap_overflow"`
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
func (q *quoteExactAmountIn) PrepareResult(ctx context.Context, scalingFactor osmomath.Dec, spotPriceCalculator domain.SpotPriceQuoteCalculator, tokenMetadataFetcher domain.TokensMetadataFetcher, logger log.Logger) ([]domain.SplitRoute, osmomath.Dec, error) {
	totalAmountIn := q.AmountIn.Amount.ToLegacyDec()
	totalFeeAcrossRoutes := osmomath.ZeroDec()

	totalLiquidityCapOverflow := false
	totalLiquidityCap := osmomath.ZeroInt()

	totalSpotPriceInBaseOutQuote := osmomath.ZeroDec()
	totalEffectiveSpotPriceInBaseOutQuote := osmomath.ZeroDec()

	denoms := []string{q.AmountIn.Denom}
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
		newPools, routeSpotPriceInBaseOutQuote, effectiveSpotPriceInBaseOutQuote, err := curRoute.PrepareResultPoolsOutGivenIn(ctx, sdk.NewCoin(q.AmountIn.Denom, amountInFraction), spotPriceCalculator, logger)
		if err != nil {
			return nil, osmomath.Dec{}, err
		}

		// Calculate total liquidity cap for the route
		for _, pool := range newPools {
			denoms = append(denoms, pool.GetTokenOutDenom())
			if cap := pool.GetLiquidityCap(); !cap.IsNil() {
				var err error
				totalLiquidityCap, err = totalLiquidityCap.SafeAdd(pool.GetLiquidityCap())
				if err != nil {
					totalLiquidityCapOverflow = true
					break
				}
			}
		}

		totalSpotPriceInBaseOutQuote = totalSpotPriceInBaseOutQuote.AddMut(routeSpotPriceInBaseOutQuote.MulMut(routeAmountInFraction))
		totalEffectiveSpotPriceInBaseOutQuote = totalEffectiveSpotPriceInBaseOutQuote.AddMut(effectiveSpotPriceInBaseOutQuote.MulMut(routeAmountInFraction))

		resultRoutes = append(resultRoutes, &route.RouteWithOutAmount{
			RouteImpl: route.RouteImpl{
				Pools:                      newPools,
				HasGeneralizedCosmWasmPool: curRoute.ContainsGeneralizedCosmWasmPool(),
			},
			InAmount:  curRoute.GetAmountIn(),
			OutAmount: curRoute.GetAmountOut(),
		})
	}

	var tokens []Token
	for denom, metadata := range tokenMetadataFetcher.GetPoolDenomsMetadata(slices.Compact(denoms)) {
		tokens = append(tokens, Token{
			Denom:                   denom,
			LiquidityCapitalization: metadata.TotalLiquidityCap,
		})
	}

	// Ensure the tokens are sorted in a consistent order
	slices.SortFunc(tokens, func(a, b Token) int {
		if a.LiquidityCapitalization.LT(b.LiquidityCapitalization) {
			return 1
		}

		if a.LiquidityCapitalization.GT(b.LiquidityCapitalization) {
			return -1
		}

		return 0
	})

	// Calculate price impact
	if !totalSpotPriceInBaseOutQuote.IsZero() {
		q.PriceImpact = totalEffectiveSpotPriceInBaseOutQuote.Quo(totalSpotPriceInBaseOutQuote).SubMut(one)
	}

	q.LiquidityCap = totalLiquidityCap
	q.LiquidityCapOverflow = totalLiquidityCapOverflow
	q.EffectiveFee = totalFeeAcrossRoutes
	q.Route = resultRoutes
	q.Tokens = tokens
	q.InBaseOutQuoteSpotPrice = totalSpotPriceInBaseOutQuote

	return q.Route, q.EffectiveFee, nil
}

// GetAmountIn implements Quote.
func (q *quoteExactAmountIn) GetAmountIn() sdk.Coin {
	return q.AmountIn
}

// GetAmountOut implements Quote.
func (q *quoteExactAmountIn) GetAmountOut() sdk.Coin {
	return sdk.Coin{Amount: q.AmountOut}
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
