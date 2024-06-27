package pools

import (
	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/sqs/domain"
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
	cosmWasmConfig domain.CosmWasmPoolRouterConfig,
	tokenOutDenom string,
	takerFee osmomath.Dec,
) (domain.RoutablePool, error) {
	return newRoutableCosmWasmPoolWithCustomModel(pool, cosmwasmPool, cosmWasmConfig, tokenOutDenom, takerFee)
}
