package grpc

import (
	"context"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/domain/workerpool"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
	prototypes "github.com/osmosis-labs/sqs/sqsdomain/proto/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type IngestGRPCHandler struct {
	logger log.Logger

	ingestUseCase mvc.IngestUsecase

	prototypes.UnimplementedSQSIngesterServer

	blockProcessDispatcher *workerpool.Dispatcher[uint64]
}

type IngestProcessBlockArgs struct {
	Pools []sqsdomain.PoolI
}

const (
	// numBlockProcessWorkers is the number of workers to process blocks concurrently
	// TODO: move to config
	numBlockProcessWorkers = 2

	tracerName = "sqs-ingest-handler"
)

var (
	tracer = otel.Tracer(tracerName)
)

var _ prototypes.SQSIngesterServer = &IngestGRPCHandler{}

// NewIngestHandler will initialize the ingest/ resources endpoint
func NewIngestGRPCHandler(us mvc.IngestUsecase, grpcIngesterConfig domain.GRPCIngesterConfig, logger log.Logger) (*grpc.Server, error) {
	ingestHandler := &IngestGRPCHandler{
		ingestUseCase:          us,
		logger:                 logger,
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

	// If there's some metadata in the context, retrieve it.
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Internal, "unable to retrieve metadata")
	}

	// Extract the existing span context from the incoming request
	parentCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(md))

	// Start a new span representing the request
	// The span ends when the request is complete
	parentCtx, span := tracer.Start(parentCtx, "IngestGRPCHandler.ProcessBlock", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

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
			ctx := context.Background()
			span := trace.SpanFromContext(parentCtx)
			ctx = trace.ContextWithSpan(ctx, span)

			if err := i.ingestUseCase.ProcessBlockData(ctx, req.BlockHeight, takerFeeMap, req.Pools); err != nil {
				// Increment error counter
				i.logger.Error(domain.SQSIngestUsecaseProcessBlockErrorMetricName, zap.Uint64("height", req.BlockHeight), zap.Error(err))
				domain.SQSIngestHandlerProcessBlockErrorCounter.Inc()

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
				i.logger.Error(domain.SQSIngestUsecaseProcessBlockErrorMetricName, zap.Uint64("height", prevResult.Result), zap.Error(prevResult.Err))
				// Increment error counter
				domain.SQSIngestHandlerProcessBlockErrorCounter.Inc()

				return prevResult.Err
			}
		default:
			// No more results in the channel, continue execution
			return nil
		}
	}
}
