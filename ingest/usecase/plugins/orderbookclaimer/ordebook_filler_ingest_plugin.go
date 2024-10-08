package orderbookclaimer

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/slices"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// orderbookClaimerIngestPlugin is a plugin that fills the orderbook orders at the end of the block.
type orderbookClaimerIngestPlugin struct {
	keyring             keyring.Keyring
	poolsUseCase        mvc.PoolsUsecase
	orderbookusecase    mvc.OrderBookUsecase
	orderbookRepository orderbookdomain.OrderBookRepository
	orderBookClient     orderbookgrpcclientdomain.OrderBookClient

	atomicBool atomic.Bool

	logger log.Logger
}

var _ domain.EndBlockProcessPlugin = &orderbookClaimerIngestPlugin{}

const (
	tracerName = "sqs-orderbook-claimer"
)

var (
	tracer = otel.Tracer(tracerName)
)

func New(
	keyring keyring.Keyring,
	orderbookusecase mvc.OrderBookUsecase,
	poolsUseCase mvc.PoolsUsecase,
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	passthroughGRPCClient passthroughdomain.PassthroughGRPCClient,
	orderBookCWAPIClient orderbookplugindomain.OrderbookCWAPIClient,
	logger log.Logger,
) *orderbookClaimerIngestPlugin {
	return &orderbookClaimerIngestPlugin{
		keyring:             keyring,
		orderbookusecase:    orderbookusecase,
		orderbookRepository: orderbookRepository,
		orderBookClient:     orderBookClient,
		poolsUseCase:        poolsUseCase,

		atomicBool: atomic.Bool{},

		logger: logger,
	}
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin.
func (o *orderbookClaimerIngestPlugin) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	ctx, span := tracer.Start(ctx, "orderbooktFillerIngestPlugin.ProcessEndBlock")
	defer span.End()

	orderbooks, err := o.poolsUseCase.GetAllCanonicalOrderbookPoolIDs()
	if err != nil {
		o.logger.Error("failed to get all canonical orderbook pool IDs", zap.Error(err))
		return err
	}

	// For simplicity, we allow only one block to be processed at a time.
	// This may be relaxed in the future.
	if !o.atomicBool.CompareAndSwap(false, true) {
		o.logger.Info("orderbook claimer is already in progress", zap.Uint64("block_height", blockHeight))
		return nil
	}
	defer o.atomicBool.Store(false)

	for _, orderbook := range orderbooks {
		// TODO: get ticks
		ticks, ok := o.orderbookRepository.GetAllTicks(orderbook.PoolID)
		if !ok {
			// TODO: report an error, this should not happen
			fmt.Printf("no ticks for orderbook %s\n", orderbook.ContractAddress)
			continue
		}

		for _, t := range ticks {
			// TODO: Do we wont to store all orders inside memory?
			orders, err := o.orderBookClient.GetOrdersByTick(ctx, orderbook.ContractAddress, t.Tick.TickId)
			if err != nil {
				fmt.Printf("no unable to fetch orderbook orders by tick	ID %d\n", t.Tick.TickId)
				continue
			}

			if len(orders) == 0 {
				continue // nothing to do
			}

			var claimable orderbookdomain.Orders
			claimable = append(claimable, o.getClaimableOrders(orderbook, orders.OrderByDirection("ask"), t.TickState.AskValues)...)
			claimable = append(claimable, o.getClaimableOrders(orderbook, orders.OrderByDirection("bid"), t.TickState.BidValues)...)
			if len(claimable) == 0 {
				continue // nothing to do
			}

			// Chunk claimable orders of size 100
			for _, chunk := range slices.Split(claimable, 100) {
				var claims []Claim
				for _, order := range chunk {
					claims = append(claims, Claim{
						TickID:  order.TickId,
						OrderID: order.OrderId,
					})
				}

				err = o.sendBatchClaimTx(orderbook.ContractAddress, claims)

				fmt.Println("claims", orderbook.ContractAddress, claims, err)
			}

			break
		}
	}

	o.logger.Info("processed end block in orderbook claimer ingest plugin", zap.Uint64("block_height", blockHeight))

	return nil
}

func (q *orderbookClaimerIngestPlugin) getClaimableOrders(orderbook domain.CanonicalOrderBooksResult, orders orderbookdomain.Orders, tickValues orderbookdomain.TickValues) orderbookdomain.Orders {
	// When cumulative total value of the tick is equal to its effective total amount swapped
	// that would mean that all orders for this tick is already filled and we attempt to claim all orders
	if tickValues.CumulativeTotalValue == tickValues.EffectiveTotalAmountSwapped {
		return orders
	}

	// Calculate claimable orders for the tick by iterating over each active order
	// and checking each orders percent filled
	var claimable orderbookdomain.Orders
	for _, order := range orders {
		result, err := q.orderbookusecase.CreateFormattedLimitOrder(
			orderbook,
			order,
		)

		// TODO: to config?
		claimable_threshold, err := osmomath.NewDecFromStr("0.98")
		if err != nil {
		}

		if result.PercentFilled.GT(claimable_threshold) && result.PercentFilled.LTE(osmomath.OneDec()) {
			claimable = append(claimable, order)
		}
	}

	return claimable
}
