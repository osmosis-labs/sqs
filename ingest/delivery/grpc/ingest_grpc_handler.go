package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	prototypes "github.com/osmosis-labs/osmosis/v31/ingest/types/proto/types"
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

	// queueMu guards pendingJobs. A job is removed from pendingJobs under this
	// lock before processing starts, so a queued job is merged into only
	// while it has not started.
	queueMu sync.Mutex

	// pendingJobs is a FIFO queue drained by a single goroutine so that
	// blocks are applied strictly in the order they were received.
	pendingJobs []*blockJob

	// jobsReady wakes the processing goroutine when a job is queued.
	jobsReady chan struct{}

	// blockErrs holds block processing errors to be surfaced on the next RPC.
	blockErrs chan blockError
}

type blockJob struct {
	ctx context.Context
	req *prototypes.ProcessBlockRequest

	takerFeeMap ingesttypes.TakerFeeMap

	// poolIndex maps a pool id to its index in req.Pools. It is built lazily
	// the first time the job is merged into.
	poolIndex map[uint64]int
}

type blockError struct {
	height uint64
	err    error
}

const (
	// blockQueueSize is the max number of blocks waiting to be processed.
	// When full, the incoming block is merged into the newest queued block
	// rather than rejected, since a failed RPC makes the node re-send all data
	// on the next block.
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
		jobsReady:     make(chan struct{}, 1),
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

	// Retained codec boundary: TakerFeeMap owns its wire format ("denom0|denom1" keys) and
	// decodes it with encoding/json internally, matching the node that encodes it.
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

	job := &blockJob{ctx: jobCtx, req: req, takerFeeMap: takerFeeMap}

	i.queueMu.Lock()
	if len(i.pendingJobs) < blockQueueSize {
		i.pendingJobs = append(i.pendingJobs, job)
		i.queueMu.Unlock()
		i.signalJobsReady()
		return &prototypes.ProcessBlockReply{}, nil
	}

	// Queue is full: merge into the newest queued block. Blocks carry per-pool
	// upserts and a taker fee upsert map, so the merge is equivalent to applying
	// both in order.
	err := mergeBlockJobs(i.pendingJobs[len(i.pendingJobs)-1], job)
	i.queueMu.Unlock()
	if err != nil {
		domain.SQSIngestHandlerProcessBlockErrorCounter.Inc()
		return nil, status.Errorf(codes.ResourceExhausted, "block processing queue is full, dropping block %d: %v", req.BlockHeight, err)
	}

	domain.SQSIngestHandlerCoalescedBlocksCounter.Inc()

	return &prototypes.ProcessBlockReply{}, nil
}

// processBlocks processes queued blocks sequentially in FIFO order.
// Runs for the lifetime of the process.
func (i *IngestGRPCHandler) processBlocks() {
	for {
		job := i.nextJob()
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

// signalJobsReady wakes the processing goroutine without blocking.
func (i *IngestGRPCHandler) signalJobsReady() {
	select {
	case i.jobsReady <- struct{}{}:
	default:
	}
}

// nextJob blocks until a job is queued and removes it from the queue.
func (i *IngestGRPCHandler) nextJob() *blockJob {
	for {
		i.queueMu.Lock()
		if len(i.pendingJobs) > 0 {
			job := i.pendingJobs[0]
			i.pendingJobs[0] = nil
			i.pendingJobs = i.pendingJobs[1:]
			i.queueMu.Unlock()
			return job
		}
		i.queueMu.Unlock()
		<-i.jobsReady
	}
}

// mergeBlockJobs merges the later block into the earlier, not yet started, job
// so that applying the result equals applying both blocks in order:
//   - a pool present in both takes the later block's data,
//   - taker fees are upserted per denom pair with the later block winning,
//   - the height becomes the later height.
//
// An error leaves the earlier job unchanged. It is returned if the later block
// is not newer, or if a pool id cannot be determined.
func mergeBlockJobs(earlier, later *blockJob) error {
	if later.req.BlockHeight <= earlier.req.BlockHeight {
		return fmt.Errorf("block height %d is not newer than queued height %d", later.req.BlockHeight, earlier.req.BlockHeight)
	}

	poolIndex := earlier.poolIndex
	if poolIndex == nil {
		var err error
		poolIndex, err = indexPools(earlier.req.Pools)
		if err != nil {
			return err
		}
	}

	laterIDs := make([]uint64, len(later.req.Pools))
	for idx, pool := range later.req.Pools {
		id, err := poolID(pool)
		if err != nil {
			return err
		}
		laterIDs[idx] = id
	}

	pools := make([]*prototypes.PoolData, len(earlier.req.Pools), len(earlier.req.Pools)+len(later.req.Pools))
	copy(pools, earlier.req.Pools)
	for idx, pool := range later.req.Pools {
		if at, ok := poolIndex[laterIDs[idx]]; ok {
			pools[at] = pool
			continue
		}
		poolIndex[laterIDs[idx]] = len(pools)
		pools = append(pools, pool)
	}

	takerFees := make(ingesttypes.TakerFeeMap, len(earlier.takerFeeMap)+len(later.takerFeeMap))
	for pair, fee := range earlier.takerFeeMap {
		takerFees[pair] = fee
	}
	for pair, fee := range later.takerFeeMap {
		takerFees[pair] = fee
	}

	merged := *later.req
	merged.Pools = pools
	earlier.req = &merged
	earlier.takerFeeMap = takerFees
	earlier.poolIndex = poolIndex
	earlier.ctx = later.ctx

	return nil
}

func indexPools(pools []*prototypes.PoolData) (map[uint64]int, error) {
	index := make(map[uint64]int, len(pools))
	for idx, pool := range pools {
		id, err := poolID(pool)
		if err != nil {
			return nil, err
		}
		index[id] = idx
	}
	return index, nil
}

// poolID reads the pool id from the chain model JSON, which carries it as a
// top-level "id" (gamm, concentrated) or "pool_id" (cosmwasm) field.
func poolID(pool *prototypes.PoolData) (uint64, error) {
	var model struct {
		ID     json.RawMessage `json:"id"`
		PoolID json.RawMessage `json:"pool_id"`
	}
	if err := json.Unmarshal(pool.ChainModel, &model); err != nil {
		return 0, fmt.Errorf("unable to read pool id: %w", err)
	}

	raw := model.ID
	if len(raw) == 0 {
		raw = model.PoolID
	}

	id, err := strconv.ParseUint(strings.Trim(string(raw), `"`), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unable to read pool id: %w", err)
	}
	return id, nil
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
