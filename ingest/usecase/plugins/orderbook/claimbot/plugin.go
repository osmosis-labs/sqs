package claimbot

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/domain/slices"
	"github.com/osmosis-labs/sqs/log"

	"github.com/osmosis-labs/osmosis/osmomath"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

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
	tracerName = "sqs-orderbook-claimbot"
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
	msgSimulator tx.MsgSimulator,
	logger log.Logger,
	chainGRPCGatewayEndpoint string,
	chainID string,
) (*claimbot, error) {
	config, err := NewConfig(keyring, orderbookusecase, poolsUsecase, msgSimulator, logger, chainGRPCGatewayEndpoint, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
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
	ctx, span := tracer.Start(ctx, "orderbookClaimbotIngestPlugin.ProcessEndBlock")
	defer span.End()

	// For simplicity, we allow only one block to be processed at a time.
	// This may be relaxed in the future.
	if !o.atomicBool.CompareAndSwap(false, true) {
		o.config.Logger.Info("already in progress", zap.Uint64("block_height", blockHeight))
		return nil
	}
	defer o.atomicBool.Store(false)
	defer o.config.Logger.Info("processed end block", zap.Uint64("block_height", blockHeight))

	orderbooks, err := getOrderbooks(o.config.PoolsUseCase, metadata)
	if err != nil {
		o.config.Logger.Warn(
			"failed to get canonical orderbook pools for block",
			zap.Uint64("block_height", blockHeight),
			zap.Error(err),
		)
		return err
	}

	account, err := o.config.AccountQueryClient.GetAccount(ctx, o.config.Keyring.GetAddress().String())
	if err != nil {
		return err
	}

	// retrieve claimable proccessedOrderbooks for the orderbooks
	proccessedOrderbooks, err := processOrderbooksAndGetClaimableOrders(
		ctx,
		o.config.OrderbookUsecase,
		fillThreshold,
		orderbooks,
	)

	if err != nil {
		o.config.Logger.Warn(
			"failed to process block orderbooks",
			zap.Error(err),
		)
		return err
	}

	for _, orderbook := range proccessedOrderbooks {
		if orderbook.Error != nil {
			o.config.Logger.Warn(
				"failed to retrieve claimable orders",
				zap.String("contract_address", orderbook.Orderbook.ContractAddress),
				zap.Error(orderbook.Error),
			)
			continue
		}

		var claimable orderbookdomain.Orders
		for _, orderbookOrder := range orderbook.Orders {
			if orderbookOrder.Error != nil {
				o.config.Logger.Warn(
					"error processing orderbook",
					zap.String("orderbook", orderbook.Orderbook.ContractAddress),
					zap.Int64("tick", orderbookOrder.Tick.Tick.TickId),
					zap.Error(err),
				)
				continue
			}

			for _, order := range orderbookOrder.Orders {
				if order.Error != nil {
					o.config.Logger.Warn(
						"unable to create orderbook limit order; marking as not claimable",
						zap.String("orderbook", orderbook.Orderbook.ContractAddress),
						zap.Int64("tick", orderbookOrder.Tick.Tick.TickId),
						zap.Error(err),
					)
					continue
				}

				claimable = append(claimable, order.Order)
			}
		}

		if err := o.processOrderbookOrders(ctx, account, orderbook.Orderbook, claimable); err != nil {
			o.config.Logger.Warn(
				"failed to process orderbook orders",
				zap.String("contract_address", orderbook.Orderbook.ContractAddress),
				zap.Error(err),
			)
		}
	}

	return nil
}

// processOrderbookOrders processes a batch of claimable orders.
func (o *claimbot) processOrderbookOrders(ctx context.Context, account *authtypes.BaseAccount, orderbook domain.CanonicalOrderBooksResult, orders orderbookdomain.Orders) error {
	if len(orders) == 0 {
		return nil
	}

	for _, chunk := range slices.Split(orders, maxBatchOfClaimableOrders) {
		if len(chunk) == 0 {
			continue
		}

		txres, err := sendBatchClaimTx(
			ctx,
			o.config.Keyring,
			o.config.MsgSimulator,
			o.config.TxServiceClient,
			o.config.ChainID,
			account,
			orderbook.ContractAddress,
			chunk,
		)

		if err != nil || (txres != nil && txres.Code != 0) {
			o.config.Logger.Info("failed sending tx",
				zap.String("orderbook contract address", orderbook.ContractAddress),
				zap.Any("orders", chunk),
				zap.Any("tx result", txres),
				zap.Error(err),
			)

			// if the tx failed, we need to fetch the account again to get the latest sequence number.
			account, err = o.config.AccountQueryClient.GetAccount(ctx, o.config.Keyring.GetAddress().String())
			if err != nil {
				return err
			}

			continue // continue processing the next batch
		}

		// Since we have a lock on block processing, that is, if block X is being processed,
		// block X+1 processing cannot start, instead of waiting for the tx to be included
		// in the block we set the sequence number here to avoid sequence number mismatch errors.
		if err := account.SetSequence(account.GetSequence() + 1); err != nil {
			o.config.Logger.Info("failed incrementing account sequence number",
				zap.String("orderbook contract address", orderbook.ContractAddress),
				zap.Error(err),
			)
		}
	}

	return nil
}
