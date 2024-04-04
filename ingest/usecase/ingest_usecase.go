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

var (
	_ mvc.IngestUsecase = &ingestUseCase{}
)

// NewIngestUsecase will create a new pools use case object
func NewIngestUsecase(logger log.Logger) (mvc.IngestUsecase, error) {
	return &ingestUseCase{
		startProcessingTime: time.Unix(0, 0),
		logger:              logger,
	}, nil
}

func (p *ingestUseCase) ProcessBlockData(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap, poolData []*types.PoolData) (err error) {
	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration", time.Since(p.startProcessingTime)))
	return errors.New("not implemented")
}
