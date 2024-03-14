package mvc

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

// RouterUsecase represent the router's usecases
type RouterUsecase interface {
	// GetOptimalQuote returns the optimal quote for the given tokenIn and tokenOutDenom.
	GetOptimalQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (domain.Quote, error)
	// GetOptimalQuoteFromConfig returns the optimal quote for the given tokenIn and tokenOutDenom from the given config.
	GetOptimalQuoteFromConfig(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, config domain.RouterConfig) (domain.Quote, error)
	// GetBestSingleRouteQuote returns the best single route quote for the given tokenIn and tokenOutDenom.
	GetBestSingleRouteQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (domain.Quote, error)
	// GetCustomDirectQuote returns the custom direct quote for the given tokenIn, tokenOutDenom and poolID.
	// It does not search for the route. It directly computes the quote for the given poolID.
	// This allows to bypass a min liquidity requirement in the router when attempting to swap over a specific pool.
	GetCustomDirectQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolID uint64) (domain.Quote, error)
	// GetCandidateRoutes returns the candidate routes for the given tokenIn and tokenOutDenom.
	GetCandidateRoutes(ctx context.Context, tokenInDenom, tokenOutDenom string) (sqsdomain.CandidateRoutes, error)
	// GetTakerFee returns the taker fee for all token pairs in a pool.
	GetTakerFee(ctx context.Context, poolID uint64) ([]sqsdomain.TakerFeeForPair, error)
	// GetPoolSpotPrice returns the spot price of a pool.
	GetPoolSpotPrice(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error)
	// GetConfig returns the config for the SQS service
	GetConfig() domain.RouterConfig
	// GetCachedCandidateRoutes returns the candidate routes for the given tokenIn and tokenOutDenom from cache.
	// It does not recompute the routes if they are not present in cache.
	// Returns error if cache is disabled.
	GetCachedCandidateRoutes(ctx context.Context, tokenInDenom, tokenOutDenom string) (sqsdomain.CandidateRoutes, error)
	// StoreRoutes stores all router state in the files locally. Used for debugging.
	StoreRouterStateFiles(ctx context.Context) error

	GetRouterState(ctx context.Context) (domain.RouterState, error)
}
