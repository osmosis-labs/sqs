package pools

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/sqsdomain"
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
	cosmWasmPoolsParams cosmwasmdomain.CosmWasmPoolsParams,
	tokenOutDenom string,
	takerFee osmomath.Dec,
) (domain.RoutablePool, error) {
	return newRoutableCosmWasmPoolWithCustomModel(pool, cosmwasmPool, cosmWasmPoolsParams, tokenOutDenom, takerFee)
}

func (r *routableAlloyTransmuterPoolImpl) CheckStaticRateLimiter(tokenInCoin sdk.Coin) error {
	return r.checkStaticRateLimiter(tokenInCoin)
}
