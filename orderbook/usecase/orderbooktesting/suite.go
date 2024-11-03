package orderbooktesting

import (
	"context"

	"github.com/stretchr/testify/assert"

	"github.com/osmosis-labs/sqs/domain"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
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

// WithOrderID sets the order ID for the order
func (o Order) WithOrderID(id int64) Order {
	o.OrderId = id
	return o
}

// WithTickID sets the tick ID for the order
func (o Order) WithTickID(id int64) Order {
	o.TickId = id
	return o
}

// LimitOrder wraps additional helper methods for testing
type LimitOrder struct {
	orderbookdomain.LimitOrder
}

// WithOrderID sets the order ID for the order
func (o LimitOrder) WithOrderID(id int64) LimitOrder {
	o.OrderId = id
	return o
}

// WithOrderbookAddress sets the orderbook address for the order
func (o LimitOrder) WithOrderbookAddress(address string) LimitOrder {
	o.OrderbookAddress = address
	return o
}

// WithQuoteAsset sets the quote asset for the order
func (o LimitOrder) WithQuoteAsset(asset orderbookdomain.Asset) LimitOrder {
	o.QuoteAsset = asset
	return o
}

// WithBaseAsset sets the base asset for the order
func (o LimitOrder) WithBaseAsset(asset orderbookdomain.Asset) LimitOrder {
	o.BaseAsset = asset
	return o
}

// OrderbookTestHelper is a helper struct for the orderbook usecase tests
type OrderbookTestHelper struct {
	routertesting.RouterTestHelper
}

// NewOrder creates a new order based on the defaultOrder.
func (s *OrderbookTestHelper) NewOrder() Order {
	return Order{defaultOrder}
}

// NewLimitOrder creates a new limit order based on the defaultLimitOrder.
func (s *OrderbookTestHelper) NewLimitOrder() LimitOrder {
	return LimitOrder{defaultLimitOrder}
}

// NewTick creates a new orderbook tick
// direction can be either "bid" or "ask" and it determines the direction of the created tick.
func (s *OrderbookTestHelper) NewTick(effectiveTotalAmountSwapped string, unrealizedCancels int64, direction string) orderbookdomain.OrderbookTick {
	s.T().Helper()

	tickValues := orderbookdomain.TickValues{
		EffectiveTotalAmountSwapped: effectiveTotalAmountSwapped,
		CumulativeTotalValue:        "1000",
	}

	tick := orderbookdomain.OrderbookTick{
		TickState:         orderbookdomain.TickState{},
		UnrealizedCancels: orderbookdomain.UnrealizedCancels{},
	}
	switch direction {
	case "bid":
		tick.TickState.BidValues = tickValues
		if unrealizedCancels != 0 {
			tick.UnrealizedCancels.BidUnrealizedCancels = osmomath.NewInt(unrealizedCancels)
		}
	case "ask":
		tick.TickState.AskValues = tickValues
		if unrealizedCancels != 0 {
			tick.UnrealizedCancels.AskUnrealizedCancels = osmomath.NewInt(unrealizedCancels)
		}
	case "all":
		tick.TickState.AskValues = tickValues
		tick.TickState.BidValues = tickValues
		if unrealizedCancels != 0 {
			tick.UnrealizedCancels.AskUnrealizedCancels = osmomath.NewInt(unrealizedCancels)
			tick.UnrealizedCancels.BidUnrealizedCancels = osmomath.NewInt(unrealizedCancels)
		}
	default:
		s.T().Fatalf("invalid direction: %s", direction)
	}

	tick.Tick = &cosmwasmpool.OrderbookTick{
		TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{},
	}

	return tick
}

// GetTickByIDFunc returns a function that returns a tick by ID
// it is useful for mocking the repository.GetTickByIDFunc.
func (s *OrderbookTestHelper) GetTickByIDFunc(tick orderbookdomain.OrderbookTick, ok bool) func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
	return func(poolID uint64, tickID int64) (orderbookdomain.OrderbookTick, bool) {
		return tick, ok
	}
}

// NewCanonicalOrderBooksResult creates a new canonical orderbooks result
func (s *OrderbookTestHelper) NewCanonicalOrderBooksResult(poolID uint64, contractAddress string) domain.CanonicalOrderBooksResult {
	return domain.CanonicalOrderBooksResult{
		Base:            "OSMO",
		Quote:           "ATOM",
		PoolID:          poolID,
		ContractAddress: contractAddress,
	}
}

// GetAllCanonicalOrderbookPoolIDsFunc returns a function that returns all canonical orderbook pool IDs
// it is useful for mocking the poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc.
func (s *OrderbookTestHelper) GetAllCanonicalOrderbookPoolIDsFunc(err error, orderbooks ...domain.CanonicalOrderBooksResult) func() ([]domain.CanonicalOrderBooksResult, error) {
	return func() ([]domain.CanonicalOrderBooksResult, error) {
		return orderbooks, err
	}
}

// GetActiveOrdersFunc returns a function that returns active orders
func (s *OrderbookTestHelper) GetActiveOrdersFunc(orders orderbookdomain.Orders, total uint64, err error) func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
	return func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
		return orders, total, err
	}
}

// GetMetadataByChainDenomFuncEmptyToken returns a function that returns an empty token useful for mocking the tokensUsecase.GetMetadataByChainDenomFunc
func (s *OrderbookTestHelper) GetMetadataByChainDenomFuncEmptyToken() func(denom string) (domain.Token, error) {
	return func(denom string) (domain.Token, error) {
		return domain.Token{}, nil
	}
}

// GetMetadataByChainDenomFunc returns a function that returns a token by chain denom useful for mocking the tokensUsecase.GetMetadataByChainDenomFunc
// If errIfNotDenom is not empty, it will return an error if the denom passed to GetMetadataByChainDenomFunc is not equal to errIfNotDenom.
// If the denom passed to GetMetadataByChainDenomFunc is empty, it will return an empty token.
// If the denom passed to GetMetadataByChainDenomFunc is equal to the quote/base asset symbol, it will return a token with the quote/base asset symbol and decimals.
func (s *OrderbookTestHelper) GetMetadataByChainDenomFunc(order LimitOrder, errIfNotDenom string) func(denom string) (domain.Token, error) {
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
