package orderbookusecase_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	cwpoolmodel "github.com/osmosis-labs/osmosis/v26/x/cosmwasmpool/model"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v26/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"

	cltypes "github.com/osmosis-labs/osmosis/v26/x/concentrated-liquidity/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/orderbook/types"
	orderbookusecase "github.com/osmosis-labs/sqs/orderbook/usecase"
	"github.com/osmosis-labs/sqs/orderbook/usecase/orderbooktesting"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// OrderbookUsecaseTestSuite is a test suite for the orderbook usecase
type OrderbookUsecaseTestSuite struct {
	orderbooktesting.OrderbookTestHelper
}

// SetupTest sets up the test suite
func TestOrderbookUsecaseTestSuite(t *testing.T) {
	suite.Run(t, new(OrderbookUsecaseTestSuite))
}

func (s *OrderbookUsecaseTestSuite) GetSpotPriceScalingFactorByDenomFunc(v int64, err error) func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
	return func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
		return osmomath.NewDec(v), err
	}
}

func (s *OrderbookUsecaseTestSuite) getTickByIDFunc(tick orderbookdomain.OrderbookTick, ok bool) func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
	return func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
		return tick, ok
	}
}

func (s *OrderbookUsecaseTestSuite) TestProcessPool() {
	withContractInfo := func(pool *mocks.MockRoutablePool) *mocks.MockRoutablePool {
		pool.CosmWasmPoolModel.ContractInfo = cosmwasmpool.ContractInfo{
			Contract: cosmwasmpool.ORDERBOOK_CONTRACT_NAME,
			Version:  cosmwasmpool.ORDERBOOK_MIN_CONTRACT_VERSION,
		}
		return pool
	}

	withTicks := func(pool *mocks.MockRoutablePool, ticks []cosmwasmpool.OrderbookTick) *mocks.MockRoutablePool {
		pool.CosmWasmPoolModel.Data = cosmwasmpool.CosmWasmPoolData{
			Orderbook: &cosmwasmpool.OrderbookData{
				Ticks: ticks,
			},
		}

		return pool
	}

	withChainModel := func(pool *mocks.MockRoutablePool, chainPoolModel poolmanagertypes.PoolI) *mocks.MockRoutablePool {
		pool.ChainPoolModel = chainPoolModel
		return pool
	}

	pool := func() *mocks.MockRoutablePool {
		return &mocks.MockRoutablePool{
			CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{},
		}
	}

	poolWithTicks := func() *mocks.MockRoutablePool {
		return withTicks(withContractInfo(pool()), []cosmwasmpool.OrderbookTick{{TickId: 1}})
	}

	poolWithChainModel := func() *mocks.MockRoutablePool {
		return withChainModel(poolWithTicks(), &cwpoolmodel.CosmWasmPool{})
	}

	testCases := []struct {
		name          string
		pool          sqsdomain.PoolI
		setupMocks    func(usecase *orderbookusecase.OrderbookUseCaseImpl, client *mocks.OrderbookGRPCClientMock, repository *mocks.OrderbookRepositoryMock)
		expectedError error
	}{
		{
			name:          "pool is nil",
			pool:          nil,
			expectedError: &types.PoolNilError{},
		},
		{
			name: "cosmWasmPoolModel is nil",
			pool: &mocks.MockRoutablePool{
				CosmWasmPoolModel: nil,
			},
			expectedError: &types.CosmWasmPoolModelNilError{},
		},
		{
			name: "pool is not an orderbook pool",
			pool: &mocks.MockRoutablePool{
				ID:                1,
				CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{},
			},
			expectedError: &types.NotAnOrderbookPoolError{},
		},
		{
			name:          "orderbook pool has no ticks, nothing to process",
			pool:          withTicks(withContractInfo(pool()), []cosmwasmpool.OrderbookTick{}),
			expectedError: nil,
		},
		{
			name: "failed to cast pool model to CosmWasmPool",
			pool: withChainModel(poolWithTicks(), &mocks.ChainPoolMock{
				ID:   1,
				Type: poolmanagertypes.Balancer,
			}),
			expectedError: &types.FailedToCastPoolModelError{},
		},
		{
			name: "failed to fetch ticks for pool",
			pool: poolWithChainModel(),
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, client *mocks.OrderbookGRPCClientMock, repository *mocks.OrderbookRepositoryMock) {
				client.FetchTicksCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error) {
					return nil, assert.AnError
				}
			},
			expectedError: &types.FetchTicksError{},
		},
		{
			name: "failed to fetch unrealized cancels for pool",
			pool: poolWithChainModel(),
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, client *mocks.OrderbookGRPCClientMock, repository *mocks.OrderbookRepositoryMock) {
				client.FetchTickUnrealizedCancelsCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error) {
					return nil, assert.AnError
				}
			},
			expectedError: &types.FetchUnrealizedCancelsError{},
		},
		{
			name: "tick ID mismatch when fetching unrealized ticks",
			pool: poolWithChainModel(),
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, client *mocks.OrderbookGRPCClientMock, repository *mocks.OrderbookRepositoryMock) {
				client.FetchTicksCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error) {
					return []orderbookdomain.Tick{
						{TickID: 1},
					}, nil
				}
				client.FetchTickUnrealizedCancelsCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error) {
					return []orderbookgrpcclientdomain.UnrealizedTickCancels{
						{TickID: 2}, // Mismatch
					}, nil
				}
			},
			expectedError: &types.TickIDMismatchError{},
		},
		{
			name: "tick ID mismatch when fetching tick states",
			pool: poolWithChainModel(),
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, client *mocks.OrderbookGRPCClientMock, repository *mocks.OrderbookRepositoryMock) {
				client.FetchTicksCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error) {
					return []orderbookdomain.Tick{
						{TickID: 2}, // Mismatched TickID
					}, nil
				}
				client.FetchTickUnrealizedCancelsCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error) {
					return []orderbookgrpcclientdomain.UnrealizedTickCancels{
						{TickID: 1},
					}, nil
				}
			},
			expectedError: &types.TickIDMismatchError{},
		},
		{
			name: "successful pool processing",
			pool: poolWithChainModel(),
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, client *mocks.OrderbookGRPCClientMock, repository *mocks.OrderbookRepositoryMock) {
				client.FetchTicksCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error) {
					return []orderbookdomain.Tick{
						{TickID: 1, TickState: orderbookdomain.TickState{
							BidValues: orderbookdomain.TickValues{
								EffectiveTotalAmountSwapped: "100",
							},
						}},
					}, nil
				}
				client.FetchTickUnrealizedCancelsCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error) {
					return []orderbookgrpcclientdomain.UnrealizedTickCancels{
						{
							TickID: 1,
							UnrealizedCancelsState: orderbookdomain.UnrealizedCancels{
								BidUnrealizedCancels: osmomath.NewInt(100),
							},
						},
					}, nil
				}
				repository.StoreTicksFunc = func(poolID uint64, ticksMap map[int64]orderbookdomain.OrderbookTick) {
					// Assume ticks are correctly stored, no need for implementation here
				}
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create instances of the mocks
			repository := mocks.OrderbookRepositoryMock{}
			tokensusecase := mocks.TokensUsecaseMock{}
			client := mocks.OrderbookGRPCClientMock{}

			// Setup the mocks according to the test case
			usecase := orderbookusecase.New(&repository, &client, nil, &tokensusecase, nil)
			if tc.setupMocks != nil {
				tc.setupMocks(usecase, &client, &repository)
			}

			// Call the method under test
			err := usecase.ProcessPool(context.Background(), tc.pool)

			// Assert the results
			if tc.expectedError != nil {
				s.Assert().Error(err)
				s.Assert().ErrorAs(err, tc.expectedError)
			} else {
				s.Assert().NoError(err)
			}
		})
	}
}

func (s *OrderbookUsecaseTestSuite) TestGetActiveOrders() {
	testCases := []struct {
		name                 string
		setupContext         func() context.Context
		setupMocks           func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock)
		address              string
		expectedError        error
		expectedOrders       []orderbookdomain.LimitOrder
		expectedIsBestEffort bool
	}{
		{
			name: "failed to get all canonical orderbook pool IDs",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return nil, assert.AnError
				}
			},
			address:       "osmo1npsku4qlqav6udkvgfk9eran4s4edzu69vzdm6",
			expectedError: &types.FailedGetAllCanonicalOrderbookPoolIDsError{},
		},
		{
			name: "context is done before processing all orderbooks",
			setupContext: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return []domain.CanonicalOrderBooksResult{
						{PoolID: 1},
					}, nil
				}
			},
			address:       "osmo1glq2duq5f4x3m88fqwecfrfcuauy8343amy5fm",
			expectedError: context.Canceled,
		},
		{
			name: "isBestEffort set to true when one orderbook is processed with best effort",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = s.GetAllCanonicalOrderbookPoolIDsFunc(nil, s.NewCanonicalOrderBooksResult(1, "A"))

				grpcclient.GetActiveOrdersCb = s.GetActiveOrdersFunc(orderbookdomain.Orders{s.NewOrder().Order}, 1, nil)

				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFuncEmptyToken()

				// Set is best effort to true
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(orderbookdomain.OrderbookTick{}, false)
			},
			address:              "osmo1777xu9gw22pham4yzssuywmxvel5wdyqkyacdw",
			expectedError:        nil,
			expectedOrders:       []orderbookdomain.LimitOrder{},
			expectedIsBestEffort: true,
		},
		{
			name: "successful retrieval of active orders",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = s.GetAllCanonicalOrderbookPoolIDsFunc(nil, s.NewCanonicalOrderBooksResult(1, "A"))

				grpcclient.GetActiveOrdersCb = s.GetActiveOrdersFunc(orderbookdomain.Orders{s.NewOrder().Order}, 1, nil)

				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFuncEmptyToken()

				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.GetSpotPriceScalingFactorByDenomFunc(1, nil)

				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
			},
			address:       "osmo1p2pq3dt5xkj39p0420p4mm9l45394xecr00299",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				s.NewLimitOrder().WithOrderbookAddress("A").LimitOrder,
			},
			expectedIsBestEffort: false,
		},
		{
			name: "successful retrieval of active orders: 3 orders returned. 1 from orderbook A, 2 from orderbook B -> 3 orders are returned as intended",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = s.GetAllCanonicalOrderbookPoolIDsFunc(
					nil,
					s.NewCanonicalOrderBooksResult(1, "A"),
					s.NewCanonicalOrderBooksResult(1, "B"),
				)

				grpcclient.GetActiveOrdersCb = func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
					if contractAddress == "A" {
						return orderbookdomain.Orders{
							s.NewOrder().WithOrderID(3).Order,
						}, 1, nil
					}
					return orderbookdomain.Orders{
						s.NewOrder().WithOrderID(1).Order,
						s.NewOrder().WithOrderID(2).Order,
					}, 2, nil
				}

				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFuncEmptyToken()

				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.NewDec(1), nil
				}

				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
			},
			address:       "osmo1p2pq3dt5xkj39p0420p4mm9l45394xecr00299",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				s.NewLimitOrder().WithOrderID(1).WithOrderbookAddress("B").LimitOrder,
				s.NewLimitOrder().WithOrderID(2).WithOrderbookAddress("B").LimitOrder,
				s.NewLimitOrder().WithOrderID(3).WithOrderbookAddress("A").LimitOrder,
			},
			expectedIsBestEffort: false,
		},
		{
			name: "successful retrieval of active orders: 2 orders returned. 1 from orderbook A, 1 from order book B. Orderbook B is not canonical -> only 1 order is returned",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = s.GetAllCanonicalOrderbookPoolIDsFunc(
					nil,
					s.NewCanonicalOrderBooksResult(1, "A"),
					s.NewCanonicalOrderBooksResult(0, "B"), // Not canonical
				)

				grpcclient.GetActiveOrdersCb = func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
					if contractAddress == "B" {
						return orderbookdomain.Orders{
							s.NewOrder().WithOrderID(2).Order,
						}, 1, nil
					}
					return orderbookdomain.Orders{
						s.NewOrder().WithOrderID(1).Order,
					}, 2, nil
				}

				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFuncEmptyToken()

				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.NewDec(1), nil
				}

				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
			},
			address:       "osmo1p2pq3dt5xkj39p0420p4mm9l45394xecr00299",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				s.NewLimitOrder().WithOrderID(1).WithOrderbookAddress("A").LimitOrder,
			},
			expectedIsBestEffort: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create instances of the mocks
			poolsUsecase := mocks.PoolsUsecaseMock{}
			orderbookrepositorysitory := mocks.OrderbookRepositoryMock{}
			client := mocks.OrderbookGRPCClientMock{}
			tokensusecase := mocks.TokensUsecaseMock{}

			// Setup the mocks according to the test case
			usecase := orderbookusecase.New(&orderbookrepositorysitory, &client, &poolsUsecase, &tokensusecase, &log.NoOpLogger{})
			if tc.setupMocks != nil {
				tc.setupMocks(usecase, &orderbookrepositorysitory, &client, &poolsUsecase, &tokensusecase)
			}

			ctx := tc.setupContext()

			// Call the method under test
			// We are not interested in the orders returned, it's tested
			// in the TestCreateFormattedLimitOrder.
			orders, isBestEffort, err := usecase.GetActiveOrders(ctx, tc.address)

			// Sort the results by order ID to make the output more deterministic
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].OrderId < orders[j].OrderId
			})

			// Assert the results
			if tc.expectedError != nil {
				s.Assert().Error(err)
				s.ErrorIsAs(err, tc.expectedError)
			} else {
				s.Assert().NoError(err)
				s.Assert().Equal(tc.expectedIsBestEffort, isBestEffort)
				s.Assert().Equal(tc.expectedOrders, orders)
			}
		})
	}
}

func (s *OrderbookUsecaseTestSuite) TestProcessOrderBookActiveOrders() {
	newLimitOrder := func() orderbooktesting.LimitOrder {
		order := s.NewLimitOrder()
		order = order.WithQuoteAsset(orderbookdomain.Asset{Symbol: "ATOM", Decimals: 6})
		order = order.WithBaseAsset(orderbookdomain.Asset{Symbol: "OSMO", Decimals: 6})
		return order
	}

	testCases := []struct {
		name                 string
		setupMocks           func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock)
		poolID               uint64
		order                orderbooktesting.LimitOrder
		ownerAddress         string
		expectedError        error
		expectedOrders       []orderbookdomain.LimitOrder
		expectedIsBestEffort bool
	}{
		{
			name: "failed to get active orders",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.GetActiveOrdersFunc(nil, 0, assert.AnError)
			},
			poolID:        1,
			order:         newLimitOrder().WithOrderID(5),
			ownerAddress:  "osmo1epp52vecttkkvs3s84c9m8s2v2jrf7gtm3jzhg",
			expectedError: &types.FailedToGetActiveOrdersError{},
		},
		{
			name: "no active orders to process",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.GetActiveOrdersFunc(nil, 0, nil)
			},
			poolID:               83,
			order:                newLimitOrder().WithOrderbookAddress("A"),
			ownerAddress:         "osmo1h5la3t4y8cljl34lsqdszklvcn053u4ryz9qr78v64rsxezyxwlsdelsdr",
			expectedError:        nil,
			expectedOrders:       nil,
			expectedIsBestEffort: false,
		},
		{
			name: "failed to get quote token metadata",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.GetActiveOrdersFunc(orderbookdomain.Orders{s.NewOrder().Order}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFunc(newLimitOrder(), "quoteToken")
			},
			poolID:        11,
			order:         newLimitOrder().WithOrderID(1),
			ownerAddress:  "osmo103l28g7r3q90d20vta2p2mz0x7qvdr3xgfwnas",
			expectedError: &types.FailedToGetMetadataError{},
		},
		{
			name: "failed to get base token metadata",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.GetActiveOrdersFunc(orderbookdomain.Orders{s.NewOrder().Order}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFunc(newLimitOrder(), "quoteToken")
			},
			poolID:        35,
			order:         newLimitOrder().WithOrderbookAddress("D"),
			ownerAddress:  "osmo1rlj2g3etczywhawuk7zh3tv8sp9edavvntn7jr",
			expectedError: &types.FailedToGetMetadataError{},
		},
		{
			name: "error on creating formatted limit order ( no error - best effort )",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.GetActiveOrdersFunc(orderbookdomain.Orders{
					s.NewOrder().WithOrderID(1).WithTickID(1).Order,
					s.NewOrder().WithOrderID(2).WithTickID(2).Order,
				}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFunc(newLimitOrder(), "")
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.GetSpotPriceScalingFactorByDenomFunc(1, nil)
				orderbookrepository.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					tick := s.NewTick("500", 100, "bid")
					if tickID == 1 {
						return tick, true
					}
					return tick, false
				}
			},
			poolID:        5,
			order:         newLimitOrder().WithOrderID(2),
			ownerAddress:  "osmo1c8udna9h9zsm44jav39g20dmtf7xjnrclpn5fw",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				newLimitOrder().WithOrderID(1).LimitOrder,
			},
			expectedIsBestEffort: true,
		},
		{
			name: "successful processing of 1 active order",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.GetActiveOrdersFunc(orderbookdomain.Orders{s.NewOrder().Order}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFunc(newLimitOrder(), "")
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.GetSpotPriceScalingFactorByDenomFunc(1, nil)
			},

			poolID:        39,
			order:         newLimitOrder().WithOrderbookAddress("B"),
			ownerAddress:  "osmo1xhkvmfyfll0303s7xm9hh8uzzwehd98tuyjpga",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				newLimitOrder().WithOrderbookAddress("B").LimitOrder,
			},
			expectedIsBestEffort: false,
		},
		{
			name: "successful processing of 2 active orders",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.GetActiveOrdersFunc(orderbookdomain.Orders{
					s.NewOrder().WithOrderID(1).Order,
					s.NewOrder().WithOrderID(2).Order,
				}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.GetMetadataByChainDenomFunc(newLimitOrder().WithBaseAsset(orderbookdomain.Asset{Symbol: "USDC", Decimals: 6}), "")
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.GetSpotPriceScalingFactorByDenomFunc(1, nil)
			},
			poolID:        93,
			order:         newLimitOrder().WithBaseAsset(orderbookdomain.Asset{Symbol: "USDC", Decimals: 6}),
			ownerAddress:  "osmo1xhkvmfyfll0303s7xm9hh8uzzwehd98tuyjpga",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				newLimitOrder().WithOrderID(1).WithBaseAsset(orderbookdomain.Asset{Symbol: "USDC", Decimals: 6}).LimitOrder,
				newLimitOrder().WithOrderID(2).WithBaseAsset(orderbookdomain.Asset{Symbol: "USDC", Decimals: 6}).LimitOrder,
			},
			expectedIsBestEffort: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create instances of the mocks
			client := mocks.OrderbookGRPCClientMock{}
			tokensusecase := mocks.TokensUsecaseMock{}
			orderbookrepository := mocks.OrderbookRepositoryMock{}

			// Setup the mocks according to the test case
			usecase := orderbookusecase.New(&orderbookrepository, &client, nil, &tokensusecase, &log.NoOpLogger{})
			if tc.setupMocks != nil {
				tc.setupMocks(usecase, &orderbookrepository, &client, &tokensusecase)
			}

			// Call the method under test
			orders, isBestEffort, err := usecase.ProcessOrderBookActiveOrders(context.Background(), domain.CanonicalOrderBooksResult{
				ContractAddress: tc.order.OrderbookAddress,
				PoolID:          tc.poolID,
				Quote:           tc.order.QuoteAsset.Symbol,
				Base:            tc.order.BaseAsset.Symbol,
			}, tc.ownerAddress)

			// Assert the results
			if tc.expectedError != nil {
				s.Assert().Error(err)
				if errors.Is(err, tc.expectedError) {
					s.Assert().ErrorIs(err, tc.expectedError)
				} else {
					s.Assert().ErrorAs(err, tc.expectedError)
				}
			} else {
				s.Assert().NoError(err)
				s.Assert().Equal(tc.expectedOrders, orders)
				s.Assert().Equal(tc.expectedIsBestEffort, isBestEffort)
			}
		})
	}
}

func (s *OrderbookUsecaseTestSuite) TestCreateFormattedLimitOrder() {
	// Generates a string that overflows when converting to osmomath.Dec
	overflowDecStr := func() string {
		return "9223372036854775808" + strings.Repeat("0", 100000)
	}

	testCases := []struct {
		name             string
		poolID           uint64
		order            orderbookdomain.Order
		quoteAsset       orderbookdomain.Asset
		baseAsset        orderbookdomain.Asset
		orderbookAddress string
		setupMocks       func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock)
		expectedError    error
		expectedOrder    orderbookdomain.LimitOrder
	}{
		{
			name: "tick not found",
			order: orderbookdomain.Order{
				TickId: 99, // Non-existent tick ID
			},
			orderbookAddress: "osmo10dl92ghwn3v44pd8w24c3htqn2mj29549zcsn06usr56ng9ppp0qe6wd0r",
			expectedError:    &types.TickForOrderbookNotFoundError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(orderbookdomain.OrderbookTick{}, false)
			},
		},
		{
			name: "error parsing quantity",
			order: orderbookdomain.Order{
				Quantity: "invalid", // Invalid quantity
			},
			orderbookAddress: "osmo1xvmtylht48gyvwe2s5rf3w6kn5g9rc4s0da0v0md82t9ldx447gsk07thg",
			expectedError:    &types.ParsingQuantityError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("6431", 935, "ask"), true)
			},
		},
		{
			name: "overflow in quantity",
			order: orderbookdomain.Order{
				Quantity:       overflowDecStr(),
				PlacedQuantity: "1500",
				Etas:           "500",
				ClaimBounty:    "10",
			},
			orderbookAddress: "osmo1rummy6vy4pfm82ctzmz4rr6fxgk0y4jf8h5s7zsadr2znwtuvq7slvl7p4",
			expectedError:    &types.ParsingQuantityError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
			},
		},
		{
			name: "error parsing placed quantity",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "invalid", // Invalid placed quantity
			},
			orderbookAddress: "osmo1pwnxmmynz4esx79qv60cshhxkuu0glmzltsaykhccnq7jmj7tvsqdumey8",
			expectedError:    &types.ParsingPlacedQuantityError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("813", 1331, "bid"), true)
			},
		},
		{
			name: "overflow in placed quantity",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: overflowDecStr(),
				Etas:           "500",
				ClaimBounty:    "10",
			},
			orderbookAddress: "osmo1z6h6etav6mfljq66vej7eqwsu4kummg9dfkvs969syw09fm0592s3fwgcs",
			expectedError:    &types.ParsingPlacedQuantityError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
			},
		},
		{
			name: "placed quantity is zero",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "0", // division by zero
			},
			orderbookAddress: "osmo1w8jm03vws7h448yvh83utd8p43j02npydy2jll0r0k7f6w7hjspsvw2u42",
			expectedError:    &types.InvalidPlacedQuantityError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("813", 1331, "bid"), true)
			},
		},
		{
			name: "error getting spot price scaling factor",
			order: orderbookdomain.Order{
				Quantity:       "931",
				PlacedQuantity: "183",
			},
			orderbookAddress: "osmo197hxw89l3gqn5ake3l5as0zh2ls6e52ata2sgq80lep0854dwe5sstljsp",
			expectedError:    &types.GettingSpotPriceScalingFactorError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("130", 13, "ask"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.GetSpotPriceScalingFactorByDenomFunc(1, assert.AnError)
			},
		},
		{
			name: "error parsing bid effective total amount swapped",
			order: orderbookdomain.Order{
				Quantity:       "136",
				PlacedQuantity: "131",
				OrderDirection: "bid",
			},
			orderbookAddress: "osmo1s552kx03vsr7ha5ck0k9tmg74gn4w72fmmjcqgr4ky3wf96wwpcqlg7vn9",
			expectedError:    &types.ParsingTickValuesError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("invalid", 13, "bid"), true)
			},
		},
		{
			name: "error parsing ask effective total amount swapped",
			order: orderbookdomain.Order{
				Quantity:       "136",
				PlacedQuantity: "131",
				OrderDirection: "ask",
			},
			orderbookAddress: "osmo1yuz6952hrcx0hadq4mgg6fq3t04d4kxhzwsfezlvvsvhq053qyys5udd8z",
			expectedError:    &types.ParsingTickValuesError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("invalid", 1, "ask"), true)
			},
		},
		{
			name: "error parsing bid unrealized cancels",
			order: orderbookdomain.Order{
				Quantity:       "103",
				PlacedQuantity: "153",
				OrderDirection: "bid",
			},
			orderbookAddress: "osmo1apmfjhycfh4cyvc7e6px4vtfwhnl5k4l0ssjq9el4rqx8kxzh2mq5gm3j9",
			expectedError:    &types.ParsingUnrealizedCancelsError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("15", 0, "bid"), true)
			},
		},
		{
			name: "error parsing ask unrealized cancels",
			order: orderbookdomain.Order{
				Quantity:       "133",
				PlacedQuantity: "313",
				OrderDirection: "ask",
			},
			orderbookAddress: "osmo17qvca7z822w5hy6jxzvaut46k44tlyk4fshx9aklkzq6prze4s9q73u4wz",
			expectedError:    &types.ParsingUnrealizedCancelsError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("13", 0, "ask"), true)
			},
		},
		{
			name: "error parsing etas",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "1500",
				OrderDirection: "bid",
				Etas:           "invalid", // Invalid ETAs
			},
			orderbookAddress: "osmo1dkqnzv7r5wgq08yaj7cxpqy766mwneec2z2agke2l59x7qxff5sqzd2y5l",
			expectedError:    &types.ParsingEtasError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("386", 830, "bid"), true)
			},
		},
		{
			name: "overflow in etas",
			order: orderbookdomain.Order{
				Quantity:       "13500",
				PlacedQuantity: "33500",
				OrderDirection: "bid",
				Etas:           overflowDecStr(), // overflow value for ETAs
				ClaimBounty:    "10",
			},
			orderbookAddress: "osmo1nkt9lwky3l3gnrdjw075u557fhzxn9ke085uxnxvtkpj6kz2asrqkd65ra",
			expectedError:    &types.ParsingEtasError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
			},
		},
		{
			name: "error converting tick to price",
			order: orderbookdomain.Order{
				TickId:         cltypes.MinCurrentTickV2 - 1, // Invalid tick ID
				Quantity:       "1000",
				PlacedQuantity: "1500",
				OrderDirection: "ask",
				Etas:           "100",
			},
			orderbookAddress: "osmo1nzpy57uftd877avsgfqjnqtsg5jhnzt8uv8mmytnku7lt76qa4lqds80nn",
			expectedError:    &types.ConvertingTickToPriceError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("190", 150, "ask"), true)
			},
		},
		{
			name: "error parsing placed_at",
			order: orderbookdomain.Order{
				TickId:         1,
				Quantity:       "1000",
				PlacedQuantity: "1500",
				OrderDirection: "ask",
				Etas:           "100",
				PlacedAt:       "invalid", // Invalid timestamp
			},
			orderbookAddress: "osmo1ewuvnvtvh5jrcve9v8txr9eqnnq9x9vq82ujct53yzt2jpc8usjsyx72sr",
			expectedError:    &types.ParsingPlacedAtError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("100", 100, "ask"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.GetSpotPriceScalingFactorByDenomFunc(10, nil)
			},
		},
		{
			name:  "successful order processing",
			order: s.NewOrder().Order,
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.GetTickByIDFunc(s.NewTick("500", 100, "bid"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.GetSpotPriceScalingFactorByDenomFunc(1, nil)
			},
			orderbookAddress: "osmo1kfct7fcu3qqc9jlxeku873p7t5vucfzw5ujn0dh97hypg24t2w6qe9q5zs",
			expectedError:    nil,
			expectedOrder:    s.NewLimitOrder().WithOrderbookAddress("osmo1kfct7fcu3qqc9jlxeku873p7t5vucfzw5ujn0dh97hypg24t2w6qe9q5zs").LimitOrder,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create instances of the mocks
			orderbookrepository := mocks.OrderbookRepositoryMock{}
			tokensusecase := mocks.TokensUsecaseMock{}

			// Setup the mocks according to the test case
			tc.setupMocks(&orderbookrepository, &tokensusecase)

			// Initialize the use case with the mocks
			usecase := orderbookusecase.New(
				&orderbookrepository,
				nil,
				nil,
				&tokensusecase,
				nil,
			)

			// Call the method under test
			result, err := usecase.CreateFormattedLimitOrder(tc.poolID, tc.order, tc.quoteAsset, tc.baseAsset, tc.orderbookAddress)

			// Assert the results
			if tc.expectedError != nil {
				s.Assert().Error(err)
				s.Assert().ErrorAs(err, tc.expectedError)
			} else {
				s.Assert().NoError(err)
				s.Assert().Equal(tc.expectedOrder, result)
			}
		})
	}
}
