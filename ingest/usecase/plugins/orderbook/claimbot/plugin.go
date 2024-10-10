package claimbot

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/delivery/grpc"
	"github.com/osmosis-labs/sqs/domain"
	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/domain/slices"
	"github.com/osmosis-labs/sqs/log"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// claimbot is a claim bot that processes and claims eligible orderbook orders at the end of each block.
// Claimable orders are determined based on order filled percentage that is handled with fillThreshold package level variable.
type claimbot struct {
	keyring             keyring.Keyring
	poolsUseCase        mvc.PoolsUsecase
	orderbookusecase    mvc.OrderBookUsecase
	orderbookRepository orderbookdomain.OrderBookRepository
	orderBookClient     orderbookgrpcclientdomain.OrderBookClient

	accountQueryClient authtypes.QueryClient
	grpcClient         *grpc.Client
	atomicBool         atomic.Bool

	logger log.Logger
}

var _ domain.EndBlockProcessPlugin = &claimbot{}

const (
	tracerName = "sqs-orderbook-claimer"
)

var (
	tracer        = otel.Tracer(tracerName)
	fillThreshold = osmomath.MustNewDecFromStr("0.98")
)

// New creates and returns a new claimbot instance.
func New(
	keyring keyring.Keyring,
	orderbookusecase mvc.OrderBookUsecase,
	poolsUseCase mvc.PoolsUsecase,
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	logger log.Logger,
) (*claimbot, error) {
	// Create a connection to the gRPC server.
	grpcClient, err := grpc.NewClient(RPC)
	if err != nil {
		return nil, err
	}

	return &claimbot{
		accountQueryClient:  authtypes.NewQueryClient(LCD),
		grpcClient:          grpcClient,
		keyring:             keyring,
		orderbookusecase:    orderbookusecase,
		orderbookRepository: orderbookRepository,
		orderBookClient:     orderBookClient,
		poolsUseCase:        poolsUseCase,

		atomicBool: atomic.Bool{},

		logger: logger,
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
		o.logger.Info("orderbook claimer is already in progress", zap.Uint64("block_height", blockHeight))
		return nil
	}
	defer o.atomicBool.Store(false)

	orderbooks, err := o.getOrderbooks(ctx, blockHeight, metadata)
	if err != nil {
		return err
	}

	// retrieve claimable orders for the orderbooks
	orders := processOrderbooksAndGetClaimableOrders(
		ctx,
		fillThreshold,
		orderbooks,
		o.orderbookRepository,
		o.orderBookClient,
		o.orderbookusecase,
		o.logger,
	)

	for _, orderbook := range orders {
		if orderbook.err != nil {
			fmt.Println("step1 error", orderbook.err)
			continue
		}

		if err := o.processBatchClaimOrders(ctx, orderbook.orderbook, orderbook.orders); err != nil {
			o.logger.Info(
				"failed to process orderbook orders",
				zap.String("contract_address", orderbook.orderbook.ContractAddress),
				zap.Error(err),
			)
		}
	}

	o.logger.Info("processed end block in orderbook claimer ingest plugin", zap.Uint64("block_height", blockHeight))

	return nil
}

func (o *claimbot) processBatchClaimOrders(ctx context.Context, orderbook domain.CanonicalOrderBooksResult, orders orderbookdomain.Orders) error {
	for _, chunk := range slices.Split(orders, 100) {
		txres, err := sendBatchClaimTx(
			ctx,
			o.keyring,
			o.grpcClient,
			o.accountQueryClient,
			orderbook.ContractAddress,
			chunk,
		)

		if err != nil {
			o.logger.Info(
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

// TODO: process only block orderbooks
func (o *claimbot) getOrderbooks(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) ([]domain.CanonicalOrderBooksResult, error) {
	orderbooks, err := o.poolsUseCase.GetAllCanonicalOrderbookPoolIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get all canonical orderbook pool IDs : %w", err)
	}
	return orderbooks, nil
}
