package orderbookgrpcclientdomain

import (
	"context"
	"fmt"

	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/domain/slices"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

// OrderBookClient is an interface for fetching orders by tick from the orderbook contract.
type OrderBookClient interface {
	// GetOrdersByTick fetches orders by tick from the orderbook contract.
	GetOrdersByTick(ctx context.Context, contractAddress string, tick int64) (orderbookdomain.Orders, error)

	// GetActiveOrders fetches active orders by owner from the orderbook contract.
	GetActiveOrders(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error)

	// GetTickUnrealizedCancels fetches unrealized cancels by tick from the orderbook contract.
	GetTickUnrealizedCancels(ctx context.Context, contractAddress string, tickIDs []int64) ([]UnrealizedTickCancels, error)

	// FetchTickUnrealizedCancels fetches the unrealized cancels for a given tick ID and contract address.
	// It returns the unrealized cancels and an error if any.
	// Errors if:
	// - failed to fetch unrealized cancels
	// - mismatch in number of unrealized cancels fetched
	FetchTickUnrealizedCancels(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]UnrealizedTickCancels, error)

	// QueryTicks fetches ticks by tickIDs from the orderbook contract.
	QueryTicks(ctx context.Context, contractAddress string, ticks []int64) ([]orderbookdomain.Tick, error)

	// FetchTicksForOrderbook fetches the ticks in chunks of maxQueryTicks at the time for a given tick ID and contract address.
	// It returns the ticks and an error if any.
	// Errors if:
	// - failed to fetch ticks
	// - mismatch in number of ticks fetched
	FetchTicks(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error)
}

// orderbookClientImpl is an implementation of OrderbookCWAPIClient.
type orderbookClientImpl struct {
	wasmClient wasmtypes.QueryClient
}

var _ OrderBookClient = (*orderbookClientImpl)(nil)

// New creates a new orderbookClientImpl.
func New(wasmClient wasmtypes.QueryClient) *orderbookClientImpl {
	return &orderbookClientImpl{
		wasmClient: wasmClient,
	}
}

// GetOrdersByTick implements OrderBookClient.
func (o *orderbookClientImpl) GetOrdersByTick(ctx context.Context, contractAddress string, tick int64) (orderbookdomain.Orders, error) {
	ordersByTick := ordersByTick{Tick: tick}

	var orders ordersByTickResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, o.wasmClient, contractAddress, ordersByTickRequest{OrdersByTick: ordersByTick}, &orders); err != nil {
		return nil, err
	}

	return orders.Orders, nil
}

// GetActiveOrders implements OrderBookClient.
func (o *orderbookClientImpl) GetActiveOrders(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
	var orders activeOrdersResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, o.wasmClient, contractAddress, activeOrdersRequest{OrdersByOwner: ordersByOwner{Owner: ownerAddress}}, &orders); err != nil {
		return nil, 0, err
	}

	return orders.Orders, orders.Count, nil
}

// GetTickUnrealizedCancels implements OrderBookClient.
func (o *orderbookClientImpl) GetTickUnrealizedCancels(ctx context.Context, contractAddress string, tickIDs []int64) ([]UnrealizedTickCancels, error) {
	var unrealizedCancels unrealizedCancelsResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, o.wasmClient, contractAddress, unrealizedCancelsByTickIdRequest{UnrealizedCancels: unrealizedCancelsRequestPayload{TickIds: tickIDs}}, &unrealizedCancels); err != nil {
		return nil, err
	}
	return unrealizedCancels.Ticks, nil
}

// FetchTickUnrealizedCancels implements OrderBookClient.
func (o *orderbookClientImpl) FetchTickUnrealizedCancels(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]UnrealizedTickCancels, error) {
	allUnrealizedCancels := make([]UnrealizedTickCancels, 0, len(tickIDs))

	for _, chunk := range slices.Split(tickIDs, chunkSize) {
		unrealizedCancels, err := o.GetTickUnrealizedCancels(ctx, contractAddress, chunk)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch unrealized cancels for ticks %v: %w", chunk, err)
		}
		allUnrealizedCancels = append(allUnrealizedCancels, unrealizedCancels...)
	}

	if len(allUnrealizedCancels) != len(tickIDs) {
		return nil, fmt.Errorf("mismatch in number of unrealized cancels fetched: expected %d, got %d", len(tickIDs), len(allUnrealizedCancels))
	}

	return allUnrealizedCancels, nil
}

// QueryTicks implements OrderBookClient.
func (o *orderbookClientImpl) QueryTicks(ctx context.Context, contractAddress string, ticks []int64) ([]orderbookdomain.Tick, error) {
	var orderbookTicks queryTicksResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, o.wasmClient, contractAddress, queryTicksRequest{TicksByID: ticksByID{TickIDs: ticks}}, &orderbookTicks); err != nil {
		return nil, err
	}
	return orderbookTicks.Ticks, nil
}

// FetchTicks implements OrderBookClient.
func (o *orderbookClientImpl) FetchTicks(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error) {
	finalTickStates := make([]orderbookdomain.Tick, 0, len(tickIDs))

	for _, chunk := range slices.Split(tickIDs, chunkSize) {
		tickStates, err := o.QueryTicks(ctx, contractAddress, chunk)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch ticks for pool %s: %w", contractAddress, err)
		}

		finalTickStates = append(finalTickStates, tickStates...)
	}

	if len(finalTickStates) != len(tickIDs) {
		return nil, fmt.Errorf("mismatch in number of ticks fetched: expected %d, got %d", len(tickIDs), len(finalTickStates))
	}

	return finalTickStates, nil
}
