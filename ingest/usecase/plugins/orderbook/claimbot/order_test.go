package claimbot_test

import (
	"context"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/claimbot"

	"github.com/stretchr/testify/assert"
)

func TestProcessOrderbooksAndGetClaimableOrders(t *testing.T) {
	newOrder := func(direction string) orderbookdomain.Order {
		return orderbookdomain.Order{
			TickId:         1,
			OrderId:        1,
			OrderDirection: direction,
		}
	}

	newCanonicalOrderBooksResult := func(poolID uint64, contractAddress string) domain.CanonicalOrderBooksResult {
		return domain.CanonicalOrderBooksResult{PoolID: poolID, ContractAddress: contractAddress}
	}

	tests := []struct {
		name           string
		fillThreshold  osmomath.Dec
		orderbooks     []domain.CanonicalOrderBooksResult
		mockSetup      func(*mocks.OrderbookUsecaseMock)
		expectedOrders []claimbot.ProcessedOrderbook
	}{
		{
			name:          "No orderbooks",
			fillThreshold: osmomath.NewDec(1),
			orderbooks:    []domain.CanonicalOrderBooksResult{},
			mockSetup: func(usecase *mocks.OrderbookUsecaseMock) {
			},
			expectedOrders: nil,
		},
		{
			name:          "Single orderbook with no claimable orders",
			fillThreshold: osmomath.NewDecWithPrec(95, 2), // 0.95
			orderbooks: []domain.CanonicalOrderBooksResult{
				newCanonicalOrderBooksResult(10, "contract1"),
			},
			mockSetup: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.WithGetClaimableOrdersForOrderbook(nil, nil)
			},
			expectedOrders: []claimbot.ProcessedOrderbook{
				{
					Orderbook: newCanonicalOrderBooksResult(10, "contract1"), // orderbook with
					Orders:    nil,                                           // no claimable orders
				},
			},
		},
		{
			name:          "Tick fully filled: all orders are claimable",
			fillThreshold: osmomath.NewDecWithPrec(99, 2), // 0.99
			orderbooks: []domain.CanonicalOrderBooksResult{
				newCanonicalOrderBooksResult(38, "contract8"),
			},
			mockSetup: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.WithGetClaimableOrdersForOrderbook(
					[]orderbookdomain.ClaimableOrderbook{
						{
							Orders: []orderbookdomain.ClaimableOrder{
								{
									Order: newOrder("bid"),
								},
							},
						},
					}, nil)
			},
			expectedOrders: []claimbot.ProcessedOrderbook{
				{
					Orderbook: newCanonicalOrderBooksResult(38, "contract8"),
					Orders: []orderbookdomain.ClaimableOrderbook{
						{
							Orders: []orderbookdomain.ClaimableOrder{
								{
									Order: newOrder("bid"),
								},
							},
						},
					},
				},
			},
		},
		{
			name:          "Orderbook with claimable orders",
			fillThreshold: osmomath.NewDecWithPrec(95, 2), // 0.95
			orderbooks: []domain.CanonicalOrderBooksResult{
				newCanonicalOrderBooksResult(64, "contract58"),
			},
			mockSetup: func(usecase *mocks.OrderbookUsecaseMock) {
				usecase.WithGetClaimableOrdersForOrderbook(
					[]orderbookdomain.ClaimableOrderbook{
						{
							Orders: []orderbookdomain.ClaimableOrder{
								{
									Order: newOrder("ask"),
								},
								{
									Order: newOrder("bid"),
								},
							},
						},
					}, nil)
			},
			expectedOrders: []claimbot.ProcessedOrderbook{
				{
					Orderbook: newCanonicalOrderBooksResult(64, "contract58"),
					Orders: []orderbookdomain.ClaimableOrderbook{
						{
							Orders: []orderbookdomain.ClaimableOrder{
								{Order: newOrder("ask")},
								{Order: newOrder("bid")},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			usecase := mocks.OrderbookUsecaseMock{}

			tt.mockSetup(&usecase)

			result, err := claimbot.ProcessOrderbooksAndGetClaimableOrders(ctx, &usecase, tt.fillThreshold, tt.orderbooks)
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedOrders, result)
		})
	}
}
