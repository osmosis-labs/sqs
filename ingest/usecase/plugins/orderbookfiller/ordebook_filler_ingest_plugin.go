package orderbookfiller

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// orderbookFillerIngestPlugin is a plugin that fills the orderbook orders at the end of the block.
type orderbookFillerIngestPlugin struct {
	poolsUseCase  mvc.PoolsUsecase
	routerUseCase mvc.RouterUsecase
	tokensUseCase mvc.TokensUsecase

	logger log.Logger
}

var _ domain.EndBlockProcessPlugin = &orderbookFillerIngestPlugin{}

func New(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, logger log.Logger) *orderbookFillerIngestPlugin {
	return &orderbookFillerIngestPlugin{
		poolsUseCase:  poolsUseCase,
		routerUseCase: routerUseCase,
		tokensUseCase: tokensUseCase,
		logger:        logger,
	}
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin.
func (o *orderbookFillerIngestPlugin) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	o.logger.Info("processing end block in orderbook filler ingest plugin", zap.Uint64("block_height", blockHeight))
	return nil
}
