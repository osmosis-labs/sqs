package pools

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

type (
	RoutableCFMMPoolImpl            = routableBalancerPoolImpl
	RoutableConcentratedPoolImpl    = routableConcentratedPoolImpl
	RoutableTransmuterPoolImpl      = routableTransmuterPoolImpl
	RoutableResultPoolImpl          = routableResultPoolImpl
	RoutableAlloyTransmuterPoolImpl = routableAlloyTransmuterPoolImpl
	RoutableOrderbookPoolImpl       = routableOrderbookPoolImpl
)

func NewRoutableCosmWasmPoolWithCustomModel(
	pool sqsdomain.PoolI,
	cosmwasmPool *cwpoolmodel.CosmWasmPool,
	cosmWasmPoolsParams CosmWasmPoolsParams,
	tokenOutDenom string,
	takerFee osmomath.Dec,
) (domain.RoutablePool, error) {
	return newRoutableCosmWasmPoolWithCustomModel(pool, cosmwasmPool, cosmWasmPoolsParams, tokenOutDenom, takerFee)
}

func (r *routableAlloyTransmuterPoolImpl) CheckStaticRateLimiter(tokenInDenom string, tokenInWeight osmomath.Dec) error {
	return r.checkStaticRateLimiter(tokenInDenom, tokenInWeight)
}

func (r *routableAlloyTransmuterPoolImpl) ComputeResultedWeights(tokenInCoin sdk.Coin) (map[string]osmomath.Dec, error) {
	return r.computeResultedWeights(tokenInCoin)
}

func CleanUpOutdatedDivision(changeLimier cosmwasmpool.ChangeLimiter, time time.Time) (*cosmwasmpool.Division, []cosmwasmpool.Division, error) {
	return cleanUpOutdatedDivision(changeLimier, time)
}
