package orderbookusecase_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	cltypes "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/types"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
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
