package orderbookusecase_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"

	cltypes "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/orderbook/types"
	orderbookusecase "github.com/osmosis-labs/sqs/orderbook/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
)

var defaultOrder = orderbookdomain.Order{
	TickId:         1,
	OrderId:        1,
	OrderDirection: "bid",
	Owner:          "owner1",
	Quantity:       "1000",
	PlacedQuantity: "1500",
	Etas:           "500",
	ClaimBounty:    "10",
	PlacedAt:       "1634764800000",
}

var defaultLimitOrder = orderbookdomain.LimitOrder{
	TickId:           1,
	OrderId:          1,
	OrderDirection:   "bid",
	Owner:            "owner1",
	Quantity:         osmomath.NewDec(1000),
	Etas:             "500",
	ClaimBounty:      "10",
	PlacedQuantity:   osmomath.NewDec(1500),
	PlacedAt:         1634,
	Price:            osmomath.MustNewDecFromStr("1.000001000000000000"),
	PercentClaimed:   osmomath.MustNewDecFromStr("0.333333333333333333"),
	TotalFilled:      osmomath.MustNewDecFromStr("600"),
	PercentFilled:    osmomath.MustNewDecFromStr("0.400000000000000000"),
	OrderbookAddress: "someOrderbookAddress",
	Status:           "partiallyFilled",
	Output:           osmomath.MustNewDecFromStr("1499.998500001499998500"),
	// QuoteAsset:       orderbookdomain.Asset{Symbol: "ATOM", Decimals: 6},
	// BaseAsset:        orderbookdomain.Asset{Symbol: "OSMO", Decimals: 6},
}

type OrderbookTestHelper struct {
	routertesting.RouterTestHelper
}

type Order struct {
	orderbookdomain.Order
}

func (o Order) withOrderID(id int64) Order {
	o.OrderId = id
	return o
}

type LimitOrder struct {
	orderbookdomain.LimitOrder
}

func (o LimitOrder) withOrderID(id int64) LimitOrder {
	o.OrderId = id
	return o
}

func (o LimitOrder) withOrderbookAddress(address string) LimitOrder {
	o.OrderbookAddress = address
	return o
}

func (s *OrderbookTestHelper) newOrder() Order {
	return Order{defaultOrder}
}

func (s *OrderbookTestHelper) newLimitOrder() LimitOrder {
	return LimitOrder{defaultLimitOrder}
}

func (s *OrderbookTestHelper) newTick(effectiveTotalAmountSwapped string, unrealizedCancels int64, direction string) orderbookdomain.OrderbookTick {
	s.T().Helper()

	tickValues := orderbookdomain.TickValues{
		EffectiveTotalAmountSwapped: effectiveTotalAmountSwapped,
	}

	tick := orderbookdomain.OrderbookTick{
		TickState:         orderbookdomain.TickState{},
		UnrealizedCancels: orderbookdomain.UnrealizedCancels{},
	}

	if direction == "bid" {
		tick.TickState.BidValues = tickValues
		if unrealizedCancels != 0 {
			tick.UnrealizedCancels.BidUnrealizedCancels = osmomath.NewInt(unrealizedCancels)
		}
	} else {
		tick.TickState.AskValues = tickValues
		if unrealizedCancels != 0 {
			tick.UnrealizedCancels.AskUnrealizedCancels = osmomath.NewInt(unrealizedCancels)
		}
	}

	return tick
}

func (s *OrderbookTestHelper) getTickByIDFunc(tick orderbookdomain.OrderbookTick, ok bool) func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
	return func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
		return tick, ok
	}
}

func (s *OrderbookTestHelper) newCanonicalOrderBooksResult(poolID uint64, contractAddress string) domain.CanonicalOrderBooksResult {
	return domain.CanonicalOrderBooksResult{
		Base:            "OSMO",
		Quote:           "ATOM",
		PoolID:          poolID,
		ContractAddress: contractAddress,
	}

}

func (s *OrderbookTestHelper) getAllCanonicalOrderbookPoolIDsFunc(err error, orderbooks ...domain.CanonicalOrderBooksResult) func() ([]domain.CanonicalOrderBooksResult, error) {
	return func() ([]domain.CanonicalOrderBooksResult, error) {
		return orderbooks, err
	}
}

type OrderbookUsecaseTestSuite struct {
	OrderbookTestHelper
}

func TestOrderbookUsecaseTestSuite(t *testing.T) {
	suite.Run(t, new(OrderbookUsecaseTestSuite))
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
			tokensUsecase := mocks.TokensUsecaseMock{}
			client := mocks.OrderbookGRPCClientMock{}

			// Setup the mocks according to the test case
			usecase := orderbookusecase.New(&repository, &client, nil, &tokensUsecase, nil)
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
	withGetAllCanonicalOrderbookPoolIDs := func(poolsUsecase *mocks.PoolsUsecaseMock) *mocks.PoolsUsecaseMock {
		poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
			return []domain.CanonicalOrderBooksResult{
				{PoolID: 1},
			}, nil
		}

		return poolsUsecase
	}

	withGetMetadataByChainDenom := func(tokensusecase *mocks.TokensUsecaseMock) *mocks.TokensUsecaseMock {
		tokensusecase.GetMetadataByChainDenomFunc = func(chainDenom string) (domain.Token, error) {
			return domain.Token{}, nil
		}
		return tokensusecase
	}

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
				withGetAllCanonicalOrderbookPoolIDs(poolsUsecase)
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
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = s.getAllCanonicalOrderbookPoolIDsFunc(nil, s.newCanonicalOrderBooksResult(1, "A"))

				grpcclient.MockGetActiveOrdersCb = func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
					return orderbookdomain.Orders{
						s.newOrder().Order,
					}, 1, nil

				}

				withGetMetadataByChainDenom(tokensusecase)

				// Set is best effort to true
				orderbookrepository.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{}, false
				}
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
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = s.getAllCanonicalOrderbookPoolIDsFunc(nil, s.newCanonicalOrderBooksResult(1, "A"))

				grpcclient.MockGetActiveOrdersCb = func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
					return orderbookdomain.Orders{
						s.newOrder().Order,
					}, 1, nil
				}

				withGetMetadataByChainDenom(tokensusecase)

				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.NewDec(1), nil
				}

				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
			},
			address:       "osmo1p2pq3dt5xkj39p0420p4mm9l45394xecr00299",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				s.newLimitOrder().withOrderbookAddress("A").LimitOrder,
			},
			expectedIsBestEffort: false,
		},
		{
			name: "successful retrieval of active orders: 3 orders returned. 1 from orderbook A, 2 from orderbook B -> 3 orders are returned as intended",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = s.getAllCanonicalOrderbookPoolIDsFunc(
					nil,
					s.newCanonicalOrderBooksResult(1, "A"),
					s.newCanonicalOrderBooksResult(1, "B"),
				)

				grpcclient.MockGetActiveOrdersCb = func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
					if contractAddress == "A" {
						return orderbookdomain.Orders{
							s.newOrder().withOrderID(3).Order,
						}, 1, nil
					}
					return orderbookdomain.Orders{
						s.newOrder().withOrderID(1).Order,
						s.newOrder().withOrderID(2).Order,
					}, 2, nil
				}

				withGetMetadataByChainDenom(tokensusecase)

				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.NewDec(1), nil
				}

				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
			},
			address:       "osmo1p2pq3dt5xkj39p0420p4mm9l45394xecr00299",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				s.newLimitOrder().withOrderID(1).withOrderbookAddress("B").LimitOrder,
				s.newLimitOrder().withOrderID(2).withOrderbookAddress("B").LimitOrder,
				s.newLimitOrder().withOrderID(3).withOrderbookAddress("A").LimitOrder,
			},
			expectedIsBestEffort: false,
		},
		{
			name: "successful retrieval of active orders: 2 orders returned. 1 from orderbook A, 1 from order book B. Orderbook B is not canonical -> only 1 order is returned",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = s.getAllCanonicalOrderbookPoolIDsFunc(
					nil,
					s.newCanonicalOrderBooksResult(1, "A"),
					s.newCanonicalOrderBooksResult(0, "B"), // Not canonical
				)

				grpcclient.MockGetActiveOrdersCb = func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
					if contractAddress == "B" {
						return orderbookdomain.Orders{
							s.newOrder().withOrderID(2).Order,
						}, 1, nil
					}
					return orderbookdomain.Orders{
						s.newOrder().withOrderID(1).Order,
					}, 2, nil
				}

				withGetMetadataByChainDenom(tokensusecase)

				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.NewDec(1), nil
				}

				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
			},
			address:       "osmo1p2pq3dt5xkj39p0420p4mm9l45394xecr00299",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				s.newLimitOrder().withOrderID(1).withOrderbookAddress("A").LimitOrder,
			},
			expectedIsBestEffort: false,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create instances of the mocks
			poolsUsecase := mocks.PoolsUsecaseMock{}
			orderbookRepository := mocks.OrderbookRepositoryMock{}
			client := mocks.OrderbookGRPCClientMock{}
			tokensusecase := mocks.TokensUsecaseMock{}

			// Setup the mocks according to the test case
			usecase := orderbookusecase.New(&orderbookRepository, &client, &poolsUsecase, &tokensusecase, &log.NoOpLogger{})
			if tc.setupMocks != nil {
				tc.setupMocks(usecase, &orderbookRepository, &client, &poolsUsecase, &tokensusecase)
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

func (s *OrderbookUsecaseTestSuite) TestCreateFormattedLimitOrder() {
	getTickByIDFunc := func(effectiveTotalAmountSwapped string, unrealizedCancels int64, direction string) func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
		return func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
			tickValues := orderbookdomain.TickValues{
				EffectiveTotalAmountSwapped: effectiveTotalAmountSwapped,
			}

			tick := orderbookdomain.OrderbookTick{
				TickState:         orderbookdomain.TickState{},
				UnrealizedCancels: orderbookdomain.UnrealizedCancels{},
			}

			if direction == "bid" {
				tick.TickState.BidValues = tickValues
				if unrealizedCancels != 0 {
					tick.UnrealizedCancels.BidUnrealizedCancels = osmomath.NewInt(unrealizedCancels)
				}
			} else {
				tick.TickState.AskValues = tickValues
				if unrealizedCancels != 0 {
					tick.UnrealizedCancels.AskUnrealizedCancels = osmomath.NewInt(unrealizedCancels)
				}
			}

			return tick, true
		}
	}

	// Creates a new osmomath.Dec from a string
	// Panics if the string is invalid
	newDecFromStr := func(str string) osmomath.Dec {
		dec, err := osmomath.NewDecFromStr(str)
		s.Require().NoError(err)
		return dec
	}

	// Generates a string that overflows when converting to osmomath.Dec
	overflowDecStr := func() string {
		return "9223372036854775808" + strings.Repeat("0", 100000)
	}

	testCases := []struct {
		name          string
		poolID        uint64
		order         orderbookdomain.Order
		quoteAsset    orderbookdomain.Asset
		baseAsset     orderbookdomain.Asset
		setupMocks    func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock)
		expectedError error
		expectedOrder orderbookdomain.LimitOrder
	}{
		{
			name: "tick not found",
			order: orderbookdomain.Order{
				TickId: 99, // Non-existent tick ID
			},
			expectedError: &types.TickForOrderbookNotFoundError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{}, false
				}
			},
		},
		{
			name: "error parsing quantity",
			order: orderbookdomain.Order{
				Quantity: "invalid", // Invalid quantity
			},
			expectedError: &types.ParsingQuantityError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("6431", 935, "ask")
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
			expectedError: &types.ParsingQuantityError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("500", 100, "bid")
			},
		},
		{
			name: "error parsing placed quantity",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "invalid", // Invalid placed quantity
			},
			expectedError: &types.ParsingPlacedQuantityError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("813", 1331, "bid")
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
			expectedError: &types.ParsingPlacedQuantityError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("500", 100, "bid")
			},
		},
		{
			name: "placed quantity is zero",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "0", // division by zero
			},
			expectedError: &types.InvalidPlacedQuantityError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("813", 1331, "bid")
			},
		},
		{
			name: "error getting spot price scaling factor",
			order: orderbookdomain.Order{
				Quantity:       "931",
				PlacedQuantity: "183",
			},
			expectedError: &types.GettingSpotPriceScalingFactorError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("130", 13, "ask")
				tokensUsecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.Dec{}, assert.AnError // Simulate an error
				}
			},
		},
		{
			name: "error parsing bid effective total amount swapped",
			order: orderbookdomain.Order{
				Quantity:       "136",
				PlacedQuantity: "131",
				OrderDirection: "bid",
			},
			expectedError: &types.ParsingTickValuesError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("invalid", 13, "bid")
			},
		},
		{
			name: "error parsing ask effective total amount swapped",
			order: orderbookdomain.Order{
				Quantity:       "136",
				PlacedQuantity: "131",
				OrderDirection: "ask",
			},
			expectedError: &types.ParsingTickValuesError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("invalid", 1, "ask")
			},
		},
		{
			name: "error parsing bid unrealized cancels",
			order: orderbookdomain.Order{
				Quantity:       "103",
				PlacedQuantity: "153",
				OrderDirection: "bid",
			},
			expectedError: &types.ParsingUnrealizedCancelsError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("15", 0, "bid")
			},
		},
		{
			name: "error parsing ask unrealized cancels",
			order: orderbookdomain.Order{
				Quantity:       "133",
				PlacedQuantity: "313",
				OrderDirection: "ask",
			},
			expectedError: &types.ParsingUnrealizedCancelsError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("13", 0, "ask")
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
			expectedError: &types.ParsingEtasError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("386", 830, "bid")
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
			expectedError: &types.ParsingEtasError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("500", 100, "bid")
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
			expectedError: &types.ConvertingTickToPriceError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("190", 150, "ask")
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
			expectedError: &types.ParsingPlacedAtError{},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("100", 100, "ask")
				tokensUsecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.NewDec(10), nil
				}
			},
		},
		{
			name: "successful order processing",
			order: orderbookdomain.Order{
				TickId:         1,
				OrderId:        1,
				OrderDirection: "bid",
				Owner:          "owner1",
				Quantity:       "1000",
				PlacedQuantity: "1500",
				Etas:           "500",
				ClaimBounty:    "10",
				PlacedAt:       "1634764800000",
			},
			quoteAsset: orderbookdomain.Asset{Symbol: "ATOM", Decimals: 6},
			baseAsset:  orderbookdomain.Asset{Symbol: "OSMO", Decimals: 6},
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = getTickByIDFunc("500", 100, "bid")
				tokensUsecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.NewDec(1), nil
				}
			},
			expectedError: nil,
			expectedOrder: orderbookdomain.LimitOrder{
				TickId:           1,
				OrderId:          1,
				OrderDirection:   "bid",
				Owner:            "owner1",
				Quantity:         osmomath.NewDec(1000),
				Etas:             "500",
				ClaimBounty:      "10",
				PlacedQuantity:   osmomath.NewDec(1500),
				PlacedAt:         1634,
				Price:            newDecFromStr("1.000001000000000000"),
				PercentClaimed:   newDecFromStr("0.333333333333333333"),
				TotalFilled:      newDecFromStr("600"),
				PercentFilled:    newDecFromStr("0.400000000000000000"),
				OrderbookAddress: "someOrderbookAddress",
				Status:           "partiallyFilled",
				Output:           newDecFromStr("1499.998500001499998500"),
				QuoteAsset:       orderbookdomain.Asset{Symbol: "ATOM", Decimals: 6},
				BaseAsset:        orderbookdomain.Asset{Symbol: "OSMO", Decimals: 6},
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create instances of the mocks
			orderbookRepo := mocks.OrderbookRepositoryMock{}
			tokensUsecase := mocks.TokensUsecaseMock{}

			// Setup the mocks according to the test case
			tc.setupMocks(&orderbookRepo, &tokensUsecase)

			// Initialize the use case with the mocks
			usecase := orderbookusecase.New(
				&orderbookRepo,
				nil,
				nil,
				&tokensUsecase,
				nil,
			)

			// Call the method under test
			result, err := usecase.CreateFormattedLimitOrder(tc.poolID, tc.order, tc.quoteAsset, tc.baseAsset, "someOrderbookAddress")

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
