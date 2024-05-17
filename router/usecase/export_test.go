package usecase

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type (
	RouterUseCaseImpl = routerUseCaseImpl

	QuoteImpl = quoteImpl

	CandidatePoolWrapper = candidatePoolWrapper
)

const (
	NoTotalValueLockedError = noTotalValueLockedError
)

func ValidateAndFilterRoutes(candidateRoutes [][]candidatePoolWrapper, tokenInDenom string, logger log.Logger) (sqsdomain.CandidateRoutes, error) {
	return validateAndFilterRoutes(candidateRoutes, tokenInDenom, logger)
}

func (r *routerUseCaseImpl) HandleRoutes(ctx context.Context, pools []sqsdomain.PoolI, tokenIn sdk.Coin, tokenOutDenom string, maxRoutes, maxPoolsPerRoute int) (candidateRoutes sqsdomain.CandidateRoutes, err error) {
	return r.handleCandidateRoutes(ctx, pools, tokenIn, tokenOutDenom, maxRoutes, maxPoolsPerRoute)
}

func EstimateAndRankSingleRouteQuote(ctx context.Context, routes []route.RouteImpl, tokenIn sdk.Coin, logger log.Logger) (domain.Quote, []RouteWithOutAmount, error) {
	return estimateAndRankSingleRouteQuote(ctx, routes, tokenIn, logger)
}

func FilterDuplicatePoolIDRoutes(rankedRoutes []route.RouteImpl) []route.RouteImpl {
	return filterDuplicatePoolIDRoutes(rankedRoutes)
}

func ConvertRankedToCandidateRoutes(rankedRoutes []route.RouteImpl) sqsdomain.CandidateRoutes {
	return convertRankedToCandidateRoutes(rankedRoutes)
}

func FormatRankedRouteCacheKey(tokenInDenom string, tokenOutDenom string, tokenIOrderOfMagnitude int) string {
	return formatRankedRouteCacheKey(tokenInDenom, tokenOutDenom, tokenIOrderOfMagnitude)
}

func FormatRouteCacheKey(tokenInDenom string, tokenOutDenom string) string {
	return formatRouteCacheKey(tokenInDenom, tokenOutDenom)
}

func FormatCandidateRouteCacheKey(tokenInDenom string, tokenOutDenom string) string {
	return formatCandidateRouteCacheKey(tokenInDenom, tokenOutDenom)
}

func SortPools(pools []sqsdomain.PoolI, transmuterCodeIDs map[uint64]struct{}, totalTVL osmomath.Int, preferredPoolIDsMap map[uint64]struct{}, logger log.Logger) []sqsdomain.PoolI {
	return sortPools(pools, transmuterCodeIDs, totalTVL, preferredPoolIDsMap, logger)
}

func GetSplitQuote(ctx context.Context, routes []route.RouteImpl, tokenIn sdk.Coin) (domain.Quote, error) {
	return getSplitQuote(ctx, routes, tokenIn)
}
