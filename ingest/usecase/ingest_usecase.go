package usecase

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/sqsdomain/proto/types"
)

type ingestUseCase struct {
	// used for tracking the time taken to process a block
	startProcessingTime time.Time
	logger              log.Logger
}

var _ mvc.IngestUsecase = &ingestUseCase{}

// NewIngestUsecase will create a new ingester use case object
func NewIngestUsecase(logger log.Logger) (mvc.IngestUsecase, error) {
	return &ingestUseCase{
		startProcessingTime: time.Unix(0, 0),
		logger:              logger,
	}, nil
}

// ProcessPoolChunk implements mvc.IngestUsecase.
func (p *ingestUseCase) ProcessPoolChunk(ctx context.Context, poolData []*types.PoolData) error {
	return errors.New("not implemented")
}

// StartBlockProcess implements mvc.IngestUsecase.
func (p *ingestUseCase) StartBlockProcess(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap) (err error) {
	p.startProcessingTime = time.Now()
	p.logger.Info("starting block processing", zap.Uint64("height", height))
	return errors.New("not implemented")
}

// EndBlockProcess implements mvc.IngestUsecase.
func (p *ingestUseCase) EndBlockProcess(ctx context.Context, height uint64) (err error) {
	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration", time.Since(p.startProcessingTime)))
	return errors.New("not implemented")
}
