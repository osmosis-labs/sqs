package usecase

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/route"
)

type (
	RouterUseCaseImpl = routerUseCaseImpl

	QuoteImpl = quoteExactAmountIn

	CandidatePoolWrapper  = candidatePoolWrapper
	CandidateRouteWrapper = candidateRouteWrapper
)

const (
	NoPoolLiquidityCapError = noPoolLiquidityCapError
)

func ValidateAndFilterRoutesOutGivenIn(candidateRoutes []candidateRouteWrapper, tokenInDenom string, logger log.Logger) (ingesttypes.CandidateRoutes, error) {
	return validateAndFilterRoutesOutGivenIn(candidateRoutes, tokenInDenom, logger)
}

func (r *routerUseCaseImpl) HandleRoutes(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, candidateRouteSearchOptions domain.CandidateRouteSearchOptions) (candidateRoutes ingesttypes.CandidateRoutes, err error) {
	return r.handleCandidateRoutes(ctx, tokenIn, tokenOutDenom, candidateRouteSearchOptions)
}

func (r *routerUseCaseImpl) EstimateAndRankSingleRouteQuoteOutGivenIn(ctx context.Context, routes []route.RouteImpl, tokenIn sdk.Coin, logger log.Logger) (domain.Quote, []RouteWithOutAmount, error) {
	return r.estimateAndRankSingleRouteQuoteOutGivenIn(ctx, routes, tokenIn, logger)
}

func (r *routerUseCaseImpl) EstimateAndRankSingleRouteQuoteInGivenOut(ctx context.Context, routes []route.RouteImpl, tokenOut sdk.Coin, logger log.Logger) (domain.Quote, []RouteWithOutAmount, error) {
	return r.estimateAndRankSingleRouteQuoteInGivenOut(ctx, routes, tokenOut, logger)
}

func FilterDuplicatePoolIDRoutes(rankedRoutes []RouteWithOutAmount) []route.RouteImpl {
	return filterAndConvertDuplicatePoolIDRankedRoutes(rankedRoutes)
}

func ConvertRankedToCandidateRoutes(rankedRoutes []route.RouteImpl) ingesttypes.CandidateRoutes {
	return convertRankedToCandidateRoutes(rankedRoutes)
}

func FormatRankedRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string, tokenIOrderOfMagnitude int) string {
	return formatRankedRouteCacheKey(method, tokenInDenom, tokenOutDenom, tokenIOrderOfMagnitude)
}

func FormatRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string) string {
	return formatRouteCacheKey(method, tokenInDenom, tokenOutDenom)
}

func FormatCandidateRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string) string {
	return formatCandidateRouteCacheKey(method, tokenInDenom, tokenOutDenom)
}

func SortPools(pools []ingesttypes.PoolI, transmuterCodeIDs map[uint64]struct{}, totalTVL osmomath.Int, preferredPoolIDsMap map[uint64]struct{}, logger log.Logger) []ingesttypes.PoolI {
	return sortPools(pools, transmuterCodeIDs, totalTVL, preferredPoolIDsMap, logger)
}

func GetSplitQuote(ctx context.Context, routes []route.RouteImpl, tokenIn sdk.Coin) (domain.Quote, error) {
	return getSplitQuote(ctx, routes, tokenIn)
}

func (r *routerUseCaseImpl) RankRoutesByDirectQuote(ctx context.Context, candidateRoutes ingesttypes.CandidateRoutes, tokenIn sdk.Coin, tokenOutDenom string, maxRoutes int) (domain.Quote, []route.RouteImpl, error) {
	return r.rankRoutesByDirectQuote(ctx, candidateRoutes, tokenIn, tokenOutDenom, maxRoutes)
}

func CutRoutesForSplits(maxSplitRoutes int, routes []route.RouteImpl) []route.RouteImpl {
	return cutRoutesForSplits(maxSplitRoutes, routes)
}

func (r *routerUseCaseImpl) SetCandidateRouteCacheToMock(method domain.TokenSwapMethod, tokenInDenom, tokenOutDenom string) {
	r.candidateRouteCache.Set(formatCandidateRouteCacheKey(method, tokenInDenom, tokenOutDenom), ingesttypes.CandidateRoutes{
		// Note: some mock dummy values
		Routes: []ingesttypes.CandidateRoute{
			{}, {},
		}}, 0)
}

func (r *routerUseCaseImpl) SetRankedRouteCacheToMock(method domain.TokenSwapMethod, tokenInDenom, tokenOutDenom string, orderOfMagnitude int) {
	r.rankedRouteCache.Set(formatRankedRouteCacheKey(method, tokenInDenom, tokenOutDenom, orderOfMagnitude), ingesttypes.CandidateRoutes{
		// Note: some mock dummy values
		Routes: []ingesttypes.CandidateRoute{
			{}, {},
		}}, 0)

}
