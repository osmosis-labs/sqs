package claimbot

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/domain/slices"
	"github.com/osmosis-labs/sqs/log"

	"github.com/osmosis-labs/osmosis/osmomath"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// claimbot is a claim bot that processes and claims eligible orderbook orders at the end of each block.
// Claimable orders are determined based on order filled percentage that is handled with fillThreshold package level variable.
type claimbot struct {
	config     *Config
	atomicBool atomic.Bool
}

var _ domain.EndBlockProcessPlugin = &claimbot{}

const (
	tracerName = "sqs-orderbook-claimer"
)

var (
	tracer        = otel.Tracer(tracerName)
	fillThreshold = osmomath.MustNewDecFromStr("0.98")
)

// maxBatchOfClaimableOrders is the maximum number of claimable orders
// that can be processed in a single batch.
const maxBatchOfClaimableOrders = 100

// New creates and returns a new claimbot instance.
func New(
	keyring keyring.Keyring,
	orderbookusecase mvc.OrderBookUsecase,
	poolsUsecase mvc.PoolsUsecase,
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	logger log.Logger,
) (*claimbot, error) {
	config, err := NewConfig(keyring, orderbookusecase, poolsUsecase, orderbookRepository, orderBookClient, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create claimbot config: %w", err)
	}

	return &claimbot{
		config:     config,
		atomicBool: atomic.Bool{},
	}, nil
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin.
// This method is called at the end of each block to process and claim eligible orderbook orders.
// ProcessEndBlock implements domain.EndBlockProcessPlugin.
func (o *claimbot) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	ctx, span := tracer.Start(ctx, "orderbooktFillerIngestPlugin.ProcessEndBlock")
	defer span.End()

	// For simplicity, we allow only one block to be processed at a time.
	// This may be relaxed in the future.
	if !o.atomicBool.CompareAndSwap(false, true) {
		o.config.Logger.Info("orderbook claimer is already in progress", zap.Uint64("block_height", blockHeight))
		return nil
	}
	defer o.atomicBool.Store(false)

	orderbooks, err := getOrderbooks(o.config.PoolsUseCase, blockHeight, metadata)
	if err != nil {
		return err
	}

	// retrieve claimable orders for the orderbooks
	orders := processOrderbooksAndGetClaimableOrders(
		ctx,
		fillThreshold,
		orderbooks,
		o.config.OrderbookRepository,
		o.config.OrderBookClient,
		o.config.OrderbookUsecase,
		o.config.Logger,
	)

	for _, orderbook := range orders {
		if orderbook.Err != nil {
			fmt.Println("step1 error", orderbook.Err)
			continue
		}

		if err := o.processOrderbookOrders(ctx, orderbook.Orderbook, orderbook.Orders); err != nil {
			o.config.Logger.Info(
				"failed to process orderbook orders",
				zap.String("contract_address", orderbook.Orderbook.ContractAddress),
				zap.Error(err),
			)
		}
	}

	o.config.Logger.Info("processed end block in orderbook claimer ingest plugin", zap.Uint64("block_height", blockHeight))

	return nil
}

// processOrderbookOrders processes a batch of claimable orders.
func (o *claimbot) processOrderbookOrders(ctx context.Context, orderbook domain.CanonicalOrderBooksResult, orders orderbookdomain.Orders) error {
	for _, chunk := range slices.Split(orders, maxBatchOfClaimableOrders) {
		if len(chunk) == 0 {
			continue
		}

		txres, err := sendBatchClaimTx(
			ctx,
			o.config.Keyring,
			o.config.AccountQueryClient,
			o.config.TxfeesClient,
			o.config.GasCalculator,
			o.config.TxServiceClient,
			orderbook.ContractAddress,
			chunk,
		)

		if err != nil {
			o.config.Logger.Info(
				"failed to sent batch claim tx",
				zap.String("contract_address", orderbook.ContractAddress),
				zap.Any("tx_result", txres),
				zap.Error(err),
			)
		}

		fmt.Println("claims", orderbook.ContractAddress, txres, chunk, err)

		// Wait for block inclusion with buffer to avoid sequence mismatch
		time.Sleep(5 * time.Second)
	}

	return nil
}