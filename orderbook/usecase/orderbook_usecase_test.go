package orderbookusecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/sqs/sqsdomain"

	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"

	cltypes "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/types"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
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
		expectedError string
	}{
		{
			name:          "pool is nil",
			pool:          nil,
			expectedError: "pool is nil when processing order book",
		},
		{
			name: "cosmWasmPoolModel is nil",
			pool: &mocks.MockRoutablePool{
				CosmWasmPoolModel: nil,
			},
			expectedError: "cw pool model is nil when processing order book",
		},
		{
			name: "pool is not an orderbook pool",
			pool: &mocks.MockRoutablePool{
				ID:                1,
				CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{},
			},
			expectedError: "pool is not an orderbook pool 1",
		},
		{
			name:          "orderbook pool has no ticks, nothing to process",
			pool:          withTicks(withContractInfo(pool()), []cosmwasmpool.OrderbookTick{}),
			expectedError: "",
		},
		{
			name: "failed to cast pool model to CosmWasmPool",
			pool: withChainModel(poolWithTicks(), &mocks.ChainPoolMock{
				ID:   1,
				Type: poolmanagertypes.Balancer,
			}),
			expectedError: "failed to cast pool model to CosmWasmPool",
		},
		{
			name: "failed to fetch ticks for pool",
			pool: poolWithChainModel(),
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, client *mocks.OrderbookGRPCClientMock, repository *mocks.OrderbookRepositoryMock) {
				client.FetchTicksCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error) {
					return nil, assert.AnError
				}
			},
			expectedError: "failed to fetch ticks for pool",
		},
		{
			name: "failed to fetch unrealized cancels for pool",
			pool: poolWithChainModel(),
			setupMocks: func(usecase *orderbookusecase.OrderbookUseCaseImpl, client *mocks.OrderbookGRPCClientMock, repository *mocks.OrderbookRepositoryMock) {
				client.FetchTickUnrealizedCancelsCb = func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error) {
					return nil, assert.AnError
				}
			},
			expectedError: "failed to fetch unrealized cancels for pool",
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
			expectedError: "tick id mismatch when fetching unrealized ticks 2 1",
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
			expectedError: "tick id mismatch when fetching tick states 2 1",
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
			expectedError: "",
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
			if tc.expectedError != "" {
				s.Assert().Error(err)
				s.Assert().Contains(err.Error(), tc.expectedError)
			} else {
				s.Assert().NoError(err)
			}
		})
	}
}
func (s *OrderbookUsecaseTestSuite) TestCreateFormattedLimitOrder() {
	testCases := []struct {
		name          string
		order         orderbookdomain.Order
		quoteAsset    orderbookdomain.Asset
		baseAsset     orderbookdomain.Asset
		setupMocks    func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock)
		expectedError string
		expectedOrder orderbookdomain.LimitOrder
	}{
		{
			name: "tick not found",
			order: orderbookdomain.Order{
				TickId: 99, // Non-existent tick ID
			},
			expectedError: "tick not found",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						Tick: &cosmwasmpool.OrderbookTick{
							TickId: tickID,
						},
					}, false
				}
			},
		},
		{
			name: "error parsing quantity",
			order: orderbookdomain.Order{
				Quantity: "invalid", // Invalid quantity
			},
			expectedError: "error parsing quantity",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						Tick: &cosmwasmpool.OrderbookTick{
							TickId: tickID,
						},
					}, true
				}
			},
		},
		{
			name: "error parsing placed quantity",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "invalid", // Invalid placed quantity
			},
			expectedError: "error parsing placed quantity",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						Tick: &cosmwasmpool.OrderbookTick{
							TickId: tickID,
						},
					}, true
				}
			},
		},
		{
			name: "error getting spot price scaling factor",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "1500",
			},
			expectedError: "error getting spot price scaling factor",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						Tick: &cosmwasmpool.OrderbookTick{
							TickId: tickID,
						},
					}, true
				}
				tokensUsecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.Dec{}, assert.AnError // Simulate an error
				}
			},
		},
		{
			name: "error parsing bid effective total amount swapped",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "1500",
				OrderDirection: "bid",
			},
			expectedError: "error parsing bid effective total amount swapped",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						TickState: orderbookdomain.TickState{
							BidValues: orderbookdomain.TickValues{
								EffectiveTotalAmountSwapped: "invalid", // Invalid value
							},
						},
					}, true
				}
			},
		},
		{
			name: "error parsing bid unrealized cancels",
			order: orderbookdomain.Order{
				Quantity:       "1000",
				PlacedQuantity: "1500",
				OrderDirection: "bid",
			},
			expectedError: "error parsing bid unrealized cancels",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						TickState: orderbookdomain.TickState{
							BidValues: orderbookdomain.TickValues{
								EffectiveTotalAmountSwapped: "100",
							},
						},
						UnrealizedCancels: orderbookdomain.UnrealizedCancels{
							// Empty is invalid value
						},
					}, true
				}
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
			expectedError: "error parsing etas",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						TickState: orderbookdomain.TickState{
							BidValues: orderbookdomain.TickValues{
								EffectiveTotalAmountSwapped: "100",
							},
						},
						UnrealizedCancels: orderbookdomain.UnrealizedCancels{
							BidUnrealizedCancels: osmomath.NewInt(100),
						},
					}, true
				}
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
			expectedError: "converting tick to price",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						TickState: orderbookdomain.TickState{
							AskValues: orderbookdomain.TickValues{
								EffectiveTotalAmountSwapped: "100",
							},
						},
						UnrealizedCancels: orderbookdomain.UnrealizedCancels{
							AskUnrealizedCancels: osmomath.NewInt(100),
						},
					}, true
				}
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
			expectedError: "error parsing placed_at",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, tokensUsecase *mocks.TokensUsecaseMock) {
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						TickState: orderbookdomain.TickState{
							AskValues: orderbookdomain.TickValues{
								EffectiveTotalAmountSwapped: "100",
							},
						},
						UnrealizedCancels: orderbookdomain.UnrealizedCancels{
							AskUnrealizedCancels: osmomath.NewInt(100),
						},
					}, true
				}
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
				orderbookRepo.GetTickByIDFunc = func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
					return orderbookdomain.OrderbookTick{
						TickState: orderbookdomain.TickState{
							BidValues: orderbookdomain.TickValues{
								EffectiveTotalAmountSwapped: "500",
							},
						},
						UnrealizedCancels: orderbookdomain.UnrealizedCancels{
							BidUnrealizedCancels: osmomath.NewInt(100),
						},
					}, true
				}
				tokensUsecase.GetSpotPriceScalingFactorByDenomFunc = func(baseDenom, quoteDenom string) (osmomath.Dec, error) {
					return osmomath.NewDec(1), nil
				}
			},
			expectedError: "",
			expectedOrder: orderbookdomain.LimitOrder{
				TickId:           1,
				OrderId:          1,
				OrderDirection:   "bid",
				Owner:            "owner1",
				Quantity:         1000,
				Etas:             "500",
				ClaimBounty:      "10",
				PlacedQuantity:   1500,
				PlacedAt:         1634,
				Price:            "1.000001000000000000",
				PercentClaimed:   "0.333333333333333333",
				TotalFilled:      600,
				PercentFilled:    "0.400000000000000000",
				OrderbookAddress: "someOrderbookAddress",
				Status:           "partiallyFilled",
				Output:           "1499.998500001499998500",
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
			result, err := usecase.CreateFormattedLimitOrder(1, tc.order, tc.quoteAsset, tc.baseAsset, "someOrderbookAddress")

			// Assert the results
			if tc.expectedError != "" {
				s.Assert().Error(err)
				s.Assert().Contains(err.Error(), tc.expectedError)
			} else {
				s.Assert().NoError(err)
				s.Assert().Equal(tc.expectedOrder, result)
			}
		})
	}
}
