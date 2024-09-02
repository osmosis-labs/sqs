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
				Quantity:       "9223372036854775808", // overflow value for int64
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
				PlacedQuantity: "9223372036854775808", // overflow value for int64
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
				Etas:           "9223372036854775808", // overflow value for int64
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
