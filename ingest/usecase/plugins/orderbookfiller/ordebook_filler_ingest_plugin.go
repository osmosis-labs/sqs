package orderbookfiller

import (
	"context"
	"sync/atomic"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// orderbookFillerIngestPlugin is a plugin that fills the orderbook orders at the end of the block.
type orderbookFillerIngestPlugin struct {
	poolsUseCase  mvc.PoolsUsecase
	routerUseCase mvc.RouterUsecase
	tokensUseCase mvc.TokensUsecase

	keyring keyring.Keyring

	logger log.Logger

	swapDone atomic.Bool
}

var _ domain.EndBlockProcessPlugin = &orderbookFillerIngestPlugin{}

func New(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, keyring keyring.Keyring, logger log.Logger) *orderbookFillerIngestPlugin {
	return &orderbookFillerIngestPlugin{
		poolsUseCase:  poolsUseCase,
		routerUseCase: routerUseCase,
		tokensUseCase: tokensUseCase,

		keyring: keyring,

		logger: logger,
	}
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin.
func (o *orderbookFillerIngestPlugin) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	if !o.swapDone.Load() {
		// TODO: Pass in route here
		_, _, err := o.swapExactAmountIn(sdk.NewCoin("uosmo", sdk.NewInt(5000)), []poolmanagertypes.SwapAmountInRoute{})
		if err != nil {
			o.logger.Error("Failed to swap", zap.Error(err))
			return nil
		}
	}

	o.logger.Info("processing end block in orderbook filler ingest plugin", zap.Uint64("block_height", blockHeight))
	return nil
}
