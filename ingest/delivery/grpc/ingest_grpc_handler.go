package grpc

import (
	"context"
	"time"

	prototypes "github.com/osmosis-labs/osmosis/v28/ingest/types/proto/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
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

	// blockJobs is a FIFO queue drained by a single goroutine so that
	// blocks are applied strictly in the order they were received.
	blockJobs chan blockJob

	// blockErrs holds block processing errors to be surfaced on the next RPC.
	blockErrs chan blockError
}

type IngestProcessBlockArgs struct {
	Pools []ingesttypes.PoolI
}

type blockJob struct {
	ctx context.Context
	req *prototypes.ProcessBlockRequest

	takerFeeMap ingesttypes.TakerFeeMap
}

type blockError struct {
	height uint64
	err    error
}

const (
	// blockQueueSize is the max number of blocks waiting to be processed.
	// When full, the RPC fails rather than blocking the node, which triggers
	// the node's fallback of re-ingesting all data on the next block.
	blockQueueSize = 8

	tracerName = "sqs-ingest-handler"
)

var (
	tracer = otel.Tracer(tracerName)
)

var _ prototypes.SQSIngesterServer = &IngestGRPCHandler{}

// NewIngestHandler will initialize the ingest/ resources endpoint
func NewIngestGRPCHandler(us mvc.IngestUsecase, grpcIngesterConfig domain.GRPCIngesterConfig, logger log.Logger) (*grpc.Server, error) {
	ingestHandler := &IngestGRPCHandler{
		ingestUseCase: us,
		logger:        logger,
		blockJobs:     make(chan blockJob, blockQueueSize),
		blockErrs:     make(chan blockError, blockQueueSize),
	}

	go ingestHandler.processBlocks()

	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(grpcIngesterConfig.MaxReceiveMsgSizeBytes), grpc.ConnectionTimeout(time.Second*time.Duration(grpcIngesterConfig.ServerConnectionTimeoutSeconds)))
	prototypes.RegisterSQSIngesterServer(grpcServer, ingestHandler)

	return grpcServer, nil
}

// ProcessChainPools implements types.IngesterServer.
func (i *IngestGRPCHandler) ProcessBlock(ctx context.Context, req *prototypes.ProcessBlockRequest) (*prototypes.ProcessBlockReply, error) {
	takerFeeMap := ingesttypes.TakerFeeMap{}

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

	// Empty error queue and return the first error encountered if any
	// THis allows to trigger the fallback mechanism, reingesting all data
	// if any error is detected. Under normal circumstances, this should not
	// be triggered.
	if err := i.emptyErrors(); err != nil {
		return nil, err
	}

	// Note that processing uses a new background context since the context
	// of the RPC call will be cancelled after the RPC call is done.
	jobCtx := trace.ContextWithSpan(context.Background(), trace.SpanFromContext(parentCtx))

	select {
	case i.blockJobs <- blockJob{ctx: jobCtx, req: req, takerFeeMap: takerFeeMap}:
	default:
		domain.SQSIngestHandlerProcessBlockErrorCounter.Inc()
		return nil, status.Errorf(codes.ResourceExhausted, "block processing queue is full, dropping block %d", req.BlockHeight)
	}

	return &prototypes.ProcessBlockReply{}, nil
}

// processBlocks processes queued blocks sequentially in FIFO order.
// Runs for the lifetime of the process.
func (i *IngestGRPCHandler) processBlocks() {
	for job := range i.blockJobs {
		height := job.req.BlockHeight

		err := i.ingestUseCase.ProcessBlockData(job.ctx, height, job.takerFeeMap, job.req.Pools)
		if err == nil {
			continue
		}

		// Increment error counter
		i.logger.Error(domain.SQSIngestUsecaseProcessBlockErrorMetricName, zap.Uint64("height", height), zap.Error(err))
		domain.SQSIngestHandlerProcessBlockErrorCounter.Inc()

		// A single pending error is enough to trigger the fallback,
		// so drop the error if the queue is already full.
		select {
		case i.blockErrs <- blockError{height: height, err: err}:
		default:
		}
	}
}

// emptyErrors drains the error queue and returns the first error encountered if any.
// If no errors are pending, it returns nil.
func (i *IngestGRPCHandler) emptyErrors() error {
	var firstErr error
	for range blockQueueSize {
		select {
		case prevResult := <-i.blockErrs:
			if firstErr == nil {
				firstErr = prevResult.err
			}
		default:
			// No more errors in the channel
			return firstErr
		}
	}
	return firstErr
}
