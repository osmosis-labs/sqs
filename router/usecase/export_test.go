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

func ValidateAndFilterRoutesOutGivenIn(candidateRoutes []candidateRouteWrapper, tokenInDenom string, logger log.Logger) (ingesttypes.CandidateRoutes, error) {
	return validateAndFilterRoutesOutGivenIn(candidateRoutes, tokenInDenom, logger)
}

func (r *routerUseCaseImpl) HandleRoutesOutGivenIn(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, candidateRouteSearchOptions domain.CandidateRouteSearchOptions) (candidateRoutes ingesttypes.CandidateRoutes, err error) {
	return r.handleCandidateRoutesOutGivenIn(ctx, tokenIn, tokenOutDenom, candidateRouteSearchOptions)
}

func (r *routerUseCaseImpl) HandleRoutesInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom string, candidateRouteSearchOptions domain.CandidateRouteSearchOptions) (candidateRoutes ingesttypes.CandidateRoutes, err error) {
	return r.handleCandidateRoutesInGivenOut(ctx, tokenOut, tokenInDenom, candidateRouteSearchOptions)
}

func (r *routerUseCaseImpl) EstimateAndRankSingleRouteQuoteOutGivenIn(ctx context.Context, routes []route.RouteImpl, tokenIn sdk.Coin, onAllRoutesFailed func()) (domain.Quote, []route.RouteWithOutAmount, error) {
	return r.estimateAndRankSingleRouteQuoteOutGivenIn(ctx, routes, tokenIn, onAllRoutesFailed)
}

func (r *routerUseCaseImpl) EstimateAndRankSingleRouteQuoteInGivenOut(ctx context.Context, routes []route.RouteImpl, tokenOut sdk.Coin, logger log.Logger, onAllRoutesFailed func()) (domain.Quote, []route.RouteWithOutAmount, error) {
	return r.estimateAndRankSingleRouteQuoteInGivenOut(ctx, routes, tokenOut, logger, onAllRoutesFailed)
}

func ValidateQuoteAmountOut(quote domain.Quote) (domain.Quote, error) {
	return validateQuoteAmountOut(quote)
}

func FilterDuplicatePoolIDRoutes(rankedRoutes []route.RouteWithOutAmount) []route.RouteImpl {
	return filterAndConvertDuplicatePoolIDRankedRoutes(rankedRoutes)
}

func ConvertRankedToCandidateRoutes(rankedRoutes []route.RouteImpl) ingesttypes.CandidateRoutes {
	return convertRankedToCandidateRoutes(rankedRoutes)
}

func FormatRankedRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string, tokenIOrderOfMagnitude int, maxSplitRoutes int) string {
	return formatRankedRouteCacheKey(method, tokenInDenom, tokenOutDenom, tokenIOrderOfMagnitude, maxSplitRoutes)
}

func FormatCandidateRouteCacheKey(method domain.TokenSwapMethod, tokenInDenom string, tokenOutDenom string) string {
	return formatCandidateRouteCacheKey(method, tokenInDenom, tokenOutDenom)
}

func (r *routerUseCaseImpl) GetQuoteMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom string) uint64 {
	return r.getQuoteMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom, r.defaultConfig.MinPoolLiquidityCap)
}

func (r *routerUseCaseImpl) GetCachedSizedCandidateRoutes(ctx context.Context, method domain.TokenSwapMethod, tokenInDenom, tokenOutDenom string, orderOfMagnitude int) (ingesttypes.CandidateRoutes, bool, error) {
	return r.getCachedCandidateRoutesByKey(ctx, formatSizedCandidateRouteCacheKey(method, tokenInDenom, tokenOutDenom, orderOfMagnitude))
}

func (r *routerUseCaseImpl) GetCandidateSearchCoin(coin sdk.Coin) sdk.Coin {
	return r.getCandidateSearchCoin(coin)
}

func SortPools(pools []ingesttypes.PoolI, transmuterCodeIDs map[uint64]struct{}, totalTVL osmomath.Int, preferredPoolIDsMap map[uint64]struct{}, logger log.Logger) []ingesttypes.PoolI {
	return sortPools(pools, transmuterCodeIDs, totalTVL, preferredPoolIDsMap, logger)
}

func GetSplitQuote(ctx context.Context, routes []route.RouteImpl, tokenIn sdk.Coin) (domain.Quote, error) {
	return getSplitQuote(ctx, routes, tokenIn)
}

func GetSplitQuoteInGivenOut(ctx context.Context, routes []route.RouteImpl, tokenOut sdk.Coin) (domain.Quote, error) {
	return getSplitQuoteInGivenOut(ctx, routes, tokenOut)
}

func (r *routerUseCaseImpl) RankRoutesByDirectQuoteOutGivenIn(ctx context.Context, candidateRoutes ingesttypes.CandidateRoutes, tokenIn sdk.Coin, tokenOutDenom string, maxRoutes int) (domain.Quote, []route.RouteImpl, error) {
	return r.rankRoutesByDirectQuoteOutGivenIn(ctx, candidateRoutes, tokenIn, tokenOutDenom, maxRoutes, nil)
}

func (r *routerUseCaseImpl) RankRoutesByDirectQuoteInGivenOut(ctx context.Context, candidateRoutes ingesttypes.CandidateRoutes, tokenOut sdk.Coin, tokenInDenom string, maxRoutes int) (domain.Quote, []route.RouteImpl, error) {
	return r.rankRoutesByDirectQuoteInGivenOut(ctx, candidateRoutes, tokenOut, tokenInDenom, maxRoutes, nil)
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

func (r *routerUseCaseImpl) SetRankedRouteCacheToMock(method domain.TokenSwapMethod, tokenInDenom, tokenOutDenom string, orderOfMagnitude int, maxSplitRoutes int) {
	r.rankedRouteCache.Set(formatRankedRouteCacheKey(method, tokenInDenom, tokenOutDenom, orderOfMagnitude, maxSplitRoutes), ingesttypes.CandidateRoutes{
		// Note: some mock dummy values
		Routes: []ingesttypes.CandidateRoute{
			{}, {},
		}}, 0)

}
