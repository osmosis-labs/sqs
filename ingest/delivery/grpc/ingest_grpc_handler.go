package grpc

import (
	"context"
	"strconv"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/domain/workerpool"
	"github.com/osmosis-labs/sqs/sqsdomain"
	prototypes "github.com/osmosis-labs/sqs/sqsdomain/proto/types"
	"google.golang.org/grpc"
)

type IngestGRPCHandler struct {
	ingestUseCase mvc.IngestUsecase

	prototypes.UnimplementedSQSIngesterServer

	blockProcessDispatcher *workerpool.Dispatcher[uint64]
}

type IngestProcessBlockArgs struct {
	Pools []sqsdomain.PoolI
}

const (
	// numBlockProcessWorkers is the number of workers to process blocks concurrently
	numBlockProcessWorkers = 2
)

var _ prototypes.SQSIngesterServer = &IngestGRPCHandler{}

// NewIngestHandler will initialize the ingest/ resources endpoint
func NewIngestGRPCHandler(us mvc.IngestUsecase, grpcIngesterConfig domain.GRPCIngesterConfig) (*grpc.Server, error) {
	ingestHandler := &IngestGRPCHandler{
		ingestUseCase: us,

		blockProcessDispatcher: workerpool.NewDispatcher[uint64](numBlockProcessWorkers),
	}

	go ingestHandler.blockProcessDispatcher.Run()

	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(grpcIngesterConfig.MaxReceiveMsgSizeBytes), grpc.ConnectionTimeout(time.Second*time.Duration(grpcIngesterConfig.ServerConnectionTimeoutSeconds)))
	prototypes.RegisterSQSIngesterServer(grpcServer, ingestHandler)

	return grpcServer, nil
}

// ProcessChainPools implements types.IngesterServer.
func (i *IngestGRPCHandler) ProcessBlock(ctx context.Context, req *prototypes.ProcessBlockRequest) (*prototypes.ProcessBlockReply, error) {
	takerFeeMap := sqsdomain.TakerFeeMap{}

	if err := takerFeeMap.UnmarshalJSON(req.TakerFeesMap); err != nil {
		return nil, err
	}

	// Empty result queue and return the first error encountered if any
	// THis allows to trigger the fallback mechanism, reingesting all data
	// if any error is detected. Under normal circumstances, this should not
	// be triggered.
	err := i.emptyResults()
	if err != nil {
		return nil, err
	}

	// Dispatch block processing
	i.blockProcessDispatcher.JobQueue <- workerpool.Job[uint64]{
		Task: func() (uint64, error) {
			// Process block data
			// Note that this executed a new background context since the parent context
			// if the RPC call will be cancelled after the RPC call is done.
			if err := i.ingestUseCase.ProcessBlockData(context.Background(), req.BlockHeight, takerFeeMap, req.Pools); err != nil {
				// Increment error counter
				domain.SQSIngestHandlerProcessBlockErrorCounter.WithLabelValues(err.Error(), strconv.FormatUint(req.BlockHeight, 10)).Inc()

				return req.BlockHeight, err
			}

			return req.BlockHeight, nil
		},
	}

	return &prototypes.ProcessBlockReply{}, nil
}

// emptyResults will empty the result queue and return the first error encountered if any.
// If no errors are encountered, it will return nil.
func (i *IngestGRPCHandler) emptyResults() error {
	// TODO: consider loop bound
	for {
		select {
		// Empty result queue and return if there are any errors
		// to trigger the fallback mechanism, reingesting all data.
		case prevResult := <-i.blockProcessDispatcher.ResultQueue:
			if prevResult.Err != nil {
				// Increment error counter
				domain.SQSIngestHandlerProcessBlockErrorCounter.WithLabelValues(prevResult.Err.Error(), strconv.FormatUint(prevResult.Result, 10)).Inc()

				return prevResult.Err
			}
		default:
			// No more results in the channel, continue execution
			return nil
		}
	}
}
