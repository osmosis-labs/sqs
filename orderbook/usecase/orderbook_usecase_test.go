package orderbookusecase_test

import (
	"context"
	"errors"
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
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v25/app/apptesting"
)

type OrderbookUsecaseTestSuite struct {
	apptesting.ConcentratedKeeperTestHelper
}

func TestOrderbookUsecaseTestSuite(t *testing.T) {
	suite.Run(t, new(OrderbookUsecaseTestSuite))
}

// Creates a new osmomath.Dec from a string
// Panics if the string is invalid
func (s *OrderbookUsecaseTestSuite) newDecFromStr(str string) osmomath.Dec {
	s.T().Helper()
	dec, err := osmomath.NewDecFromStr(str)
	s.Require().NoError(err)
	return dec
}

func (s *OrderbookUsecaseTestSuite) newOrder() orderbookdomain.Order {
	s.T().Helper()

	return orderbookdomain.Order{
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
}

func (s *OrderbookUsecaseTestSuite) newLimitOrder() orderbookdomain.LimitOrder {
	s.T().Helper()

	return orderbookdomain.LimitOrder{
		TickId:           1,
		OrderId:          1,
		OrderDirection:   "bid",
		Owner:            "owner1",
		Quantity:         osmomath.NewDec(1000),
		Etas:             "500",
		ClaimBounty:      "10",
		PlacedQuantity:   osmomath.NewDec(1500),
		PlacedAt:         1634,
		Price:            s.newDecFromStr("1.000001000000000000"),
		PercentClaimed:   s.newDecFromStr("0.333333333333333333"),
		TotalFilled:      s.newDecFromStr("600"),
		PercentFilled:    s.newDecFromStr("0.400000000000000000"),
		OrderbookAddress: "someOrderbookAddress",
		Status:           "partiallyFilled",
		Output:           s.newDecFromStr("1499.998500001499998500"),
		QuoteAsset:       orderbookdomain.Asset{Symbol: "ATOM", Decimals: 6},
		BaseAsset:        orderbookdomain.Asset{Symbol: "OSMO", Decimals: 6},
	}
}

func (s *OrderbookUsecaseTestSuite) newTick(effectiveTotalAmountSwapped string, unrealizedCancels int64, direction string) orderbookdomain.OrderbookTick {
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

func (s *OrderbookUsecaseTestSuite) getActiveOrdersFunc(orders orderbookdomain.Orders, total uint64, err error) func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
	return func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
		return orders, total, err
	}
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
		expectedError        error
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
			expectedError: context.Canceled,
		},
		{
			name: "processOrderBookActiveOrders returns an error",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				withGetAllCanonicalOrderbookPoolIDs(poolsUsecase)

				grpcclient.GetActiveOrdersCb = s.getActiveOrdersFunc(nil, 0, assert.AnError)
			},
			expectedError:        &types.FailedProcessingOrderbookActiveOrdersError{},
			expectedIsBestEffort: false,
		},
		{
			name: "isBestEffort set to true when one orderbook is processed with best effort",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				withGetAllCanonicalOrderbookPoolIDs(poolsUsecase)

				grpcclient.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder()}, 1, nil)

				withGetMetadataByChainDenom(tokensusecase)

				// Set is best effort to true
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(orderbookdomain.OrderbookTick{}, false)
			},
			expectedError:        nil,
			expectedIsBestEffort: true,
		},
		{
			name: "successful retrieval of active orders",
			setupContext: func() context.Context {
				return context.Background()
			},
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, grpcclient *mocks.OrderbookGRPCClientMock, poolsUsecase *mocks.PoolsUsecaseMock, tokensusecase *mocks.TokensUsecaseMock) {
				withGetAllCanonicalOrderbookPoolIDs(poolsUsecase)

				grpcclient.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder()}, 1, nil)

				withGetMetadataByChainDenom(tokensusecase)

				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, nil)

				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("100", 100, "bid"), true)
			},
			expectedError:        nil,
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
			_, isBestEffort, err := usecase.GetActiveOrders(ctx, "address")

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
				s.Assert().Equal(tc.expectedIsBestEffort, isBestEffort)
			}
		})
	}
}

func (s *OrderbookUsecaseTestSuite) TestProcessOrderBookActiveOrders() {
	withGetMetadataByChainDenom := func(tokensusecase *mocks.TokensUsecaseMock, errIfNotDenom string) *mocks.TokensUsecaseMock {
		tokensusecase.GetMetadataByChainDenomFunc = func(denom string) (domain.Token, error) {
			order := s.newLimitOrder()

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
		return tokensusecase
	}

	testCases := []struct {
		name                 string
		setupMocks           func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock)
		expectedError        error
		expectedOrders       []orderbookdomain.LimitOrder
		expectedIsBestEffort bool
	}{
		{
			name: "failed to get active orders",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(nil, 0, assert.AnError)
			},
			expectedError: &types.FailedToGetActiveOrdersError{},
		},
		{
			name: "no active orders to process",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(nil, 0, nil)
			},
			expectedError:        nil,
			expectedOrders:       nil,
			expectedIsBestEffort: false,
		},
		{
			name: "failed to get quote token metadata",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder()}, 1, nil)
				withGetMetadataByChainDenom(tokensusecase, "quoteToken")
			},
			expectedError: &types.FailedToGetMetadataError{},
		},
		{
			name: "failed to get base token metadata",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder()}, 1, nil)
				withGetMetadataByChainDenom(tokensusecase, "quoteToken")
			},
			expectedError: &types.FailedToGetMetadataError{},
		},
		{
			name: "error on creating formatted limit order ( no error - best effort )",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder()}, 1, nil)
				withGetMetadataByChainDenom(tokensusecase, "")
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(orderbookdomain.OrderbookTick{}, false)
			},
			expectedOrders:       []orderbookdomain.LimitOrder{},
			expectedError:        nil,
			expectedIsBestEffort: true,
		},
		{
			name: "successful processing of active orders",
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, orderbookrepository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, tokensusecase *mocks.TokensUsecaseMock) {
				client.GetActiveOrdersCb = s.getActiveOrdersFunc(orderbookdomain.Orders{s.newOrder()}, 1, nil)
				withGetMetadataByChainDenom(tokensusecase, "")
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, nil)
			},
			expectedError: nil,
			expectedOrders: []orderbookdomain.LimitOrder{
				s.newLimitOrder(),
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

			order := s.newLimitOrder()

			// Call the method under test
			orders, isBestEffort, err := usecase.ProcessOrderBookActiveOrders(context.Background(), domain.CanonicalOrderBooksResult{
				ContractAddress: order.OrderbookAddress,
				PoolID:          1,
				Quote:           order.QuoteAsset.Symbol,
				Base:            order.BaseAsset.Symbol,
			}, "ownerAddress")

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
				Etas:           "9223372036854775808", // overflow value for int64
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
			name:       "successful order processing",
			order:      s.newOrder(),
			quoteAsset: orderbookdomain.Asset{Symbol: "ATOM", Decimals: 6},
			baseAsset:  orderbookdomain.Asset{Symbol: "OSMO", Decimals: 6},
			setupMocks: func(orderbookrepository *mocks.OrderbookRepositoryMock, tokensusecase *mocks.TokensUsecaseMock) {
				orderbookrepository.GetTickByIDFunc = s.getTickByIDFunc(s.newTick("500", 100, "bid"), true)
				tokensusecase.GetSpotPriceScalingFactorByDenomFunc = s.getSpotPriceScalingFactorByDenomFunc(1, nil)
			},
			expectedError: nil,
			expectedOrder: s.newLimitOrder(),
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
