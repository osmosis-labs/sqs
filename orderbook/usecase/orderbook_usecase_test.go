package orderbookusecase_test

import (
	"context"
	"errors"
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

// defaultOrder is a default order used for testing
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

// defaultLimitOrder is a default limit order used for testing
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
}

// Order is a wrapper around orderbookdomain.Order
// it wraps additional helper methods for testing
type Order struct {
	orderbookdomain.Order
}

// withOrderID sets the order ID for the order
func (o Order) withOrderID(id int64) Order {
	o.OrderId = id
	return o
}

// withTickID sets the tick ID for the order
func (o Order) withTickID(id int64) Order {
	o.TickId = id
	return o
}

// withOrderbookAddress sets the orderbook address for the order
// it wraps additional helper methods for testing
type LimitOrder struct {
	orderbookdomain.LimitOrder
}

// withOrderID sets the order ID for the order
func (o LimitOrder) withOrderID(id int64) LimitOrder {
	o.OrderId = id
	return o
}

// withOrderbookAddress sets the orderbook address for the order
func (o LimitOrder) withOrderbookAddress(address string) LimitOrder {
	o.OrderbookAddress = address
	return o
}

// withQuoteAsset sets the quote asset for the order
func (o LimitOrder) withQuoteAsset(asset orderbookdomain.Asset) LimitOrder {
	o.QuoteAsset = asset
	return o
}

// withBaseAsset sets the base asset for the order
func (o LimitOrder) withBaseAsset(asset orderbookdomain.Asset) LimitOrder {
	o.BaseAsset = asset
	return o
}

// OrderbookTestHelper is a helper struct for the orderbook usecase tests
type OrderbookTestHelper struct {
	routertesting.RouterTestHelper
}

// newOrder creates a new order
func (s *OrderbookTestHelper) newOrder() Order {
	return Order{defaultOrder}
}

// newLimitOrder creates a new limit order
func (s *OrderbookTestHelper) newLimitOrder() LimitOrder {
	return LimitOrder{defaultLimitOrder}
}

// newTick creates a new orderbook tick
// direction can be either "bid" or "ask" and it determines the direction of the created tick.
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

// getTickByIDFunc returns a function that returns a tick by ID
// it is useful for mocking the repository.GetTickByIDFunc.
func (s *OrderbookTestHelper) getTickByIDFunc(tick orderbookdomain.OrderbookTick, ok bool) func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
	return func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
		return tick, ok
	}
}

// newCanonicalOrderBooksResult creates a new canonical orderbooks result
func (s *OrderbookTestHelper) newCanonicalOrderBooksResult(poolID uint64, contractAddress string) domain.CanonicalOrderBooksResult {
	return domain.CanonicalOrderBooksResult{
		Base:            "OSMO",
		Quote:           "ATOM",
		PoolID:          poolID,
		ContractAddress: contractAddress,
	}

}

// getAllCanonicalOrderbookPoolIDsFunc returns a function that returns all canonical orderbook pool IDs
// it is useful for mocking the poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc.
func (s *OrderbookTestHelper) getAllCanonicalOrderbookPoolIDsFunc(err error, orderbooks ...domain.CanonicalOrderBooksResult) func() ([]domain.CanonicalOrderBooksResult, error) {
	return func() ([]domain.CanonicalOrderBooksResult, error) {
		return orderbooks, err
	}
}

// getActiveOrdersFunc returns a function that returns active orders
func (s *OrderbookTestHelper) getActiveOrdersFunc(orders orderbookdomain.Orders, total uint64, err error) func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
	return func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
		return orders, total, err
	}
}

// getMetadataByChainDenomFunc returns a function that returns a token by chain denom
func (s *OrderbookTestHelper) getMetadataByChainDenomFunc(order LimitOrder, errIfNotDenom string) func(denom string) (domain.Token, error) {
	return func(denom string) (domain.Token, error) {
		if errIfNotDenom != "" && errIfNotDenom != denom {
			return domain.Token{}, assert.AnError
		}
		if denom == "" {
			return domain.Token{}, nil
		}

		if denom == order.QuoteAsset.Symbol {
			return domain.Token{
				CoinMinimalDenom: order.QuoteAsset.Symbol,
				Precision:        order.QuoteAsset.Decimals,
			}, nil
		}

		if denom == order.BaseAsset.Symbol {
			return domain.Token{
				CoinMinimalDenom: order.BaseAsset.Symbol,
				Precision:        order.BaseAsset.Decimals,
			}, nil
		}

		return domain.Token{}, assert.AnError
	}
}

// OrderbookUsecaseTestSuite is a test suite for the orderbook usecase
type OrderbookUsecaseTestSuite struct {
	OrderbookTestHelper
}

// SetupTest sets up the test suite
func TestOrderbookUsecaseTestSuite(t *testing.T) {
	suite.Run(t, new(OrderbookUsecaseTestSuite))
}

func (s *OrderbookUsecaseTestSuite) getSpotPriceScalingFactorByDenomFunc(v int64, err error) func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
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

				grpcclient.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder().Order}, 1, nil)

				withGetMetadataByChainDenom(tokensusecase)

				// Set is best effort to true
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(orderbookdomain.OrderbookTick{}, false)
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

				grpcclient.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder().Order}, 1, nil)

				withGetMetadataByChainDenom(tokensusecase)

				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, nil)

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

				grpcclient.GetActiveOrdersCb = func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
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

				grpcclient.GetActiveOrdersCb = func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
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
	newLimitOrder := func() LimitOrder {
		order := s.newLimitOrder()
		order = order.withQuoteAsset(orderbookdomain.Asset{Symbol: "ATOM", Decimals: 6})
		order = order.withBaseAsset(orderbookdomain.Asset{Symbol: "OSMO", Decimals: 6})
		return order
	}

	testCases := []struct {
		name                 string
		setupMocks           func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock)
		poolID               uint64
		order                LimitOrder
		ownerAddress         string
		expectedError        error
		expectedOrders       []orderbookdomain.LimitOrder
		expectedIsBestEffort bool
	}{
		{
			name: "failed to get active orders",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(nil, 0, assert.AnError)
			},
			poolID:        1,
			order:         newLimitOrder().withOrderID(5),
			ownerAddress:  "osmo1epp52vecttkkvs3s84c9m8s2v2jrf7gtm3jzhg",
			expectedError: &types.FailedToGetActiveOrdersError{},
		},
		{
			name: "no active orders to process",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(nil, 0, nil)
			},
			poolID:               83,
			order:                newLimitOrder().withOrderbookAddress("A"),
			ownerAddress:         "osmo1h5la3t4y8cljl34lsqdszklvcn053u4ryz9qr78v64rsxezyxwlsdelsdr",
			expectedError:        nil,
			expectedOrders:       nil,
			expectedIsBestEffort: false,
		},
		{
			name: "failed to get quote token metadata",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder().Order}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.getMetadataByChainDenomFunc(newLimitOrder(), "quoteToken")
			},
			poolID:        11,
			order:         newLimitOrder().withOrderID(1),
			ownerAddress:  "osmo103l28g7r3q90d20vta2p2mz0x7qvdr3xgfwnas",
			expectedError: &types.FailedToGetMetadataError{},
		},
		{
			name: "failed to get base token metadata",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder().Order}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.getMetadataByChainDenomFunc(newLimitOrder(), "quoteToken")
			},
			poolID:        35,
			order:         newLimitOrder().withOrderbookAddress("D"),
			ownerAddress:  "osmo1rlj2g3etczywhawuk7zh3tv8sp9edavvntn7jr",
			expectedError: &types.FailedToGetMetadataError{},
		},
		{
			name: "error on creating formatted limit order ( no error - best effort )",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{
					s.newOrder().withOrderID(1).withTickID(1).Order,
					s.newOrder().withOrderID(2).withTickID(2).Order,
				}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.getMetadataByChainDenomFunc(newLimitOrder(), "")
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, nil)
				orderbookrepository.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					tick := s.newTick("500", 100, "bid")
					if tickID == 1 {
						return tick, true
					}
					return tick, false
				}
			},
			poolID:        5,
			order:         newLimitOrder().withOrderID(2),
			ownerAddress:  "osmo1c8udna9h9zsm44jav39g20dmtf7xjnrclpn5fw",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				newLimitOrder().withOrderID(1).LimitOrder,
			},
			expectedIsBestEffort: true,
		},
		{
			name: "successful processing of 1 active order",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder().Order}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.getMetadataByChainDenomFunc(newLimitOrder(), "")
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, nil)
			},

			poolID:        39,
			order:         newLimitOrder().withOrderbookAddress("B"),
			ownerAddress:  "osmo1xhkvmfyfll0303s7xm9hh8uzzwehd98tuyjpga",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				newLimitOrder().withOrderbookAddress("B").LimitOrder,
			},
			expectedIsBestEffort: false,
		},
		{
			name: "successful processing of 2 active orders",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{
					s.newOrder().withOrderID(1).Order,
					s.newOrder().withOrderID(2).Order,
				}, 1, nil)
				tokensusecase.GetMetadataByChainDenomFunc = s.getMetadataByChainDenomFunc(newLimitOrder().withBaseAsset(orderbookdomain.Asset{Symbol: "USDC", Decimals: 6}), "")
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, nil)
			},
			poolID:        93,
			order:         newLimitOrder().withBaseAsset(orderbookdomain.Asset{Symbol: "USDC", Decimals: 6}),
			ownerAddress:  "osmo1xhkvmfyfll0303s7xm9hh8uzzwehd98tuyjpga",
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				newLimitOrder().withOrderID(1).withBaseAsset(orderbookdomain.Asset{Symbol: "USDC", Decimals: 6}).LimitOrder,
				newLimitOrder().withOrderID(2).withBaseAsset(orderbookdomain.Asset{Symbol: "USDC", Decimals: 6}).LimitOrder,
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
		name          string
		poolID        uint64
		order         orderbookdomain.Order
		quoteAsset    orderbookdomain.Asset
		baseAsset     orderbookdomain.Asset
		setupMocks    func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock)
		expectedError error
		expectedOrder orderbookdomain.LimitOrder
	}{
		{
			name: "tick not found",
			order: orderbookdomain.Order{
				TickId: 99, // Non-existent tick ID
			},
			expectedError: &types.TickForOrderbookNotFoundError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(orderbookdomain.OrderbookTick{}, false)
			},
		},
		{
			name: "error parsing quantity",
			order: orderbookdomain.Order{
				Quantity: "invalid", // Invalid quantity
			},
			expectedError: &types.ParsingQuantityError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("6431", 935, "ask"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
			},
		},
		{
			name: "error parsing placed quantity",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "invalid", // Invalid placed quantity
			},
			expectedError: &types.ParsingPlacedQuantityError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("813", 1331, "bid"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
			},
		},
		{
			name: "placed quantity is zero",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "0", // division by zero
			},
			expectedError: &types.InvalidPlacedQuantityError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("813", 1331, "bid"), true)
			},
		},
		{
			name: "error getting spot price scaling factor",
			order: orderbookdomain.Order{
				Quantity:       "931",
				PlacedQuantity: "183",
			},
			expectedError: &types.GettingSpotPriceScalingFactorError{},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("130", 13, "ask"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, assert.AnError)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("invalid", 13, "bid"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("invalid", 1, "ask"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("15", 0, "bid"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("13", 0, "ask"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("386", 830, "bid"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("190", 150, "ask"), true)
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
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("100", 100, "ask"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(10, nil)
			},
		},
		{
			name:  "successful order processing",
			order: s.newOrder().Order,
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, nil)
			},
			expectedError: nil,
			expectedOrder: s.newLimitOrder().LimitOrder,
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
