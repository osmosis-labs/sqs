package pools_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
	"github.com/stretchr/testify/require"
)

func TestNewRoutableCosmWasmPoolWithCustomModel(t *testing.T) {
	alloyTransmuterCosmWasmPool := cwpoolmodel.CosmWasmPool{CodeId: 1, PoolId: 122}
	alloyTransmuterModel := cosmwasmpool.CosmWasmPoolModel{
		ContractInfo: cosmwasmpool.ContractInfo{
			Contract: cosmwasmpool.ALLOY_TRANSMUTER_CONTRACT_NAME,
			Version:  cosmwasmpool.ALLOY_TRANSMUTER_MIN_CONTRACT_VERSION,
		},
		Data: cosmwasmpool.CosmWasmPoolData{
			AlloyTransmuter: &cosmwasmpool.AlloyTransmuterData{
				AlloyedDenom: "allBTC",
				AssetConfigs: []cosmwasmpool.TransmuterAssetConfig{
					{Denom: "nbtc", NormalizationFactor: osmomath.OneInt()},
					{Denom: "allBTC", NormalizationFactor: osmomath.OneInt()},
				},
			},
		},
	}
	alloyTransmuterBalances := types.NewCoins(types.NewCoin("nbtc", types.NewInt(100000000)))
	alloyTransmuterSpreadFactor := osmomath.NewDec(0)
	alloyTransmuterTakerFee := osmomath.NewDecWithPrec(1, 2)

	orderbookCosmWasmPool := cwpoolmodel.CosmWasmPool{CodeId: 2, PoolId: 145}
	orderbookModel := cosmwasmpool.CosmWasmPoolModel{
		ContractInfo: cosmwasmpool.ContractInfo{
			Contract: cosmwasmpool.ORDERBOOK_CONTRACT_NAME,
			Version:  cosmwasmpool.ORDERBOOK_MIN_CONTRACT_VERSION,
		},
		Data: cosmwasmpool.CosmWasmPoolData{
			Orderbook: &cosmwasmpool.OrderbookData{
				QuoteDenom:       "quote",
				BaseDenom:        "base",
				NextBidTickIndex: 1,
				NextAskTickIndex: 2,
				Ticks: []cosmwasmpool.OrderbookTick{
					{TickId: 1, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{BidLiquidity: osmomath.NewBigDec(1000), AskLiquidity: osmomath.NewBigDec(500)}},
					{TickId: 2, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{BidLiquidity: osmomath.NewBigDec(1500), AskLiquidity: osmomath.NewBigDec(700)}},
				},
			},
		},
	}
	orderbookBalances := types.NewCoins(types.NewCoin("base", types.NewInt(100000000)))
	orderbookSpreadFactor := osmomath.NewDecWithPrec(1, 2)
	orderbookTakerFee := osmomath.NewDecWithPrec(2, 2)

	tests := []struct {
		name                 string
		pool                 sqsdomain.PoolI
		cosmwasmPool         *cwpoolmodel.CosmWasmPool
		cosmWasmConfig       domain.CosmWasmPoolRouterConfig
		tokenOutDenom        string
		takerFee             osmomath.Dec
		expectedRoutablePool sqsdomain.RoutablePool
		expectedError        error
	}{
		{
			name: "AlloyTransmuter with correct data",
			pool: mockPoolWithModel(alloyTransmuterCosmWasmPool.PoolId, &sqsdomain.SQSPool{
				SpreadFactor:      alloyTransmuterSpreadFactor,
				Balances:          alloyTransmuterBalances,
				CosmWasmPoolModel: &alloyTransmuterModel,
			}),
			cosmwasmPool: &alloyTransmuterCosmWasmPool,
			cosmWasmConfig: domain.CosmWasmPoolRouterConfig{
				AlloyedTransmuterCodeIDs: map[uint64]struct{}{
					alloyTransmuterCosmWasmPool.CodeId: {},
				},
			},
			tokenOutDenom: "allBTC",
			takerFee:      alloyTransmuterTakerFee,
			expectedRoutablePool: &pools.RouteableAlloyTransmuterPoolImpl{
				ChainPool:           &alloyTransmuterCosmWasmPool,
				AlloyTransmuterData: alloyTransmuterModel.Data.AlloyTransmuter,
				Balances:            alloyTransmuterBalances,
				TokenOutDenom:       "allBTC",
				TakerFee:            alloyTransmuterTakerFee,
				SpreadFactor:        alloyTransmuterSpreadFactor,
			},
		},
		{
			name: "AlloyTransmuter with missing data",
			pool: mockPoolWithModel(alloyTransmuterCosmWasmPool.PoolId, &sqsdomain.SQSPool{
				CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{
					ContractInfo: cosmwasmpool.ContractInfo{
						Contract: cosmwasmpool.ALLOY_TRANSMUTER_CONTRACT_NAME,
						Version:  cosmwasmpool.ALLOY_TRANSMUTER_MIN_CONTRACT_VERSION,
					},
					Data: cosmwasmpool.CosmWasmPoolData{
						AlloyTransmuter: nil,
					},
				},
			}),
			cosmwasmPool: &alloyTransmuterCosmWasmPool,
			cosmWasmConfig: domain.CosmWasmPoolRouterConfig{
				AlloyedTransmuterCodeIDs: map[uint64]struct{}{
					alloyTransmuterCosmWasmPool.CodeId: {},
				},
			},
			tokenOutDenom: "token",
			takerFee:      alloyTransmuterTakerFee,
			expectedError: domain.CosmWasmPoolDataMissingError{
				CosmWasmPoolType: domain.CosmWasmPoolAlloyTransmuter,
				PoolId:           alloyTransmuterCosmWasmPool.PoolId,
			},
		},
		{
			name: "Orderbook with correct data",
			pool: mockPoolWithModel(orderbookCosmWasmPool.PoolId, &sqsdomain.SQSPool{
				SpreadFactor:      orderbookSpreadFactor,
				Balances:          orderbookBalances,
				CosmWasmPoolModel: &orderbookModel,
			}),
			cosmwasmPool: &orderbookCosmWasmPool,
			cosmWasmConfig: domain.CosmWasmPoolRouterConfig{
				OrderbookCodeIDs: map[uint64]struct{}{
					orderbookCosmWasmPool.CodeId: {},
				},
			},
			tokenOutDenom: "quote",
			takerFee:      orderbookTakerFee,
			expectedRoutablePool: &pools.RouteableOrderbookPoolImpl{
				ChainPool:     &orderbookCosmWasmPool,
				OrderbookData: orderbookModel.Data.Orderbook,
				Balances:      orderbookBalances,
				TokenOutDenom: "quote",
				TakerFee:      orderbookTakerFee,
				SpreadFactor:  orderbookSpreadFactor,
			},
		},
		{
			name: "Orderbook with missing data",
			pool: mockPoolWithModel(orderbookCosmWasmPool.PoolId, &sqsdomain.SQSPool{
				CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{
					ContractInfo: cosmwasmpool.ContractInfo{
						Contract: cosmwasmpool.ORDERBOOK_CONTRACT_NAME,
						Version:  cosmwasmpool.ORDERBOOK_MIN_CONTRACT_VERSION,
					},
					Data: cosmwasmpool.CosmWasmPoolData{
						Orderbook: nil,
					},
				},
			}),
			cosmwasmPool: &orderbookCosmWasmPool,
			cosmWasmConfig: domain.CosmWasmPoolRouterConfig{
				OrderbookCodeIDs: map[uint64]struct{}{
					orderbookCosmWasmPool.CodeId: {},
				},
			},
			tokenOutDenom: "token",
			takerFee:      osmomath.NewDec(1),
			expectedError: domain.CosmWasmPoolDataMissingError{
				CosmWasmPoolType: domain.CosmWasmPoolOrderbook,
				PoolId:           orderbookCosmWasmPool.PoolId,
			},
		},
		{
			name: "Unsupported pool type",
			pool: mockPoolWithModel(1, &sqsdomain.SQSPool{}),
			cosmwasmPool: &cwpoolmodel.CosmWasmPool{
				CodeId: 3,
				PoolId: 124,
			},
			cosmWasmConfig: domain.CosmWasmPoolRouterConfig{},
			tokenOutDenom:  "token",
			takerFee:       osmomath.NewDec(1),
			expectedError: domain.UnsupportedCosmWasmPoolError{
				PoolId: 124,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routablePool, err := pools.NewRoutableCosmWasmPoolWithCustomModel(tt.pool, tt.cosmwasmPool, tt.cosmWasmConfig, tt.tokenOutDenom, tt.takerFee)

			if tt.expectedError != nil {
				require.Equal(t, tt.expectedError, err)
			} else {
				require.Equal(t, tt.expectedRoutablePool, routablePool)
			}
		})
	}
}

// mockPoolWithModel is a helper function to create a mock pool with a given model
func mockPoolWithModel(poolId uint64, sqsPool *sqsdomain.SQSPool) sqsdomain.PoolI {
	return mockPool{
		poolId:  poolId,
		sqsPool: sqsPool,
	}
}

// mockPool is a mock implementation of sqsdomain.PoolI
type mockPool struct {
	poolId  uint64
	sqsPool *sqsdomain.SQSPool
}

func (m mockPool) GetId() uint64 {
	return m.poolId
}

func (m mockPool) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.PoolType(poolmanagertypes.PoolType_value["CosmWasm"])
}

func (m mockPool) GetPoolLiquidityCap() osmomath.Int {
	return m.sqsPool.PoolLiquidityCap
}

func (m mockPool) GetPoolDenoms() []string {
	return m.sqsPool.PoolDenoms
}

func (m mockPool) GetUnderlyingPool() poolmanagertypes.PoolI {
	panic("unimplemented")
}

func (m mockPool) GetSQSPoolModel() sqsdomain.SQSPool {
	return *m.sqsPool
}

func (m mockPool) GetTickModel() (*sqsdomain.TickModel, error) {
	panic("unimplemented")
}

func (m mockPool) SetTickModel(tickModel *sqsdomain.TickModel) error {
	return nil
}

func (m mockPool) Validate(minUOSMOTVL osmomath.Int) error {
	panic("unimplemented")
}
