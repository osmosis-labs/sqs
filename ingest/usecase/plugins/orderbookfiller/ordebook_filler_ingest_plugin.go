package orderbookfiller

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// orderbookFillerIngestPlugin is a plugin that fills the orderbook orders at the end of the block.
type orderbookFillerIngestPlugin struct {
	poolsUseCase  mvc.PoolsUsecase
	routerUseCase mvc.RouterUsecase
	tokensUseCase mvc.TokensUsecase

	passthroughGRPCClient passthroughdomain.PassthroughGRPCClient

	// TODO: set
	// orderbookCWAAPIClient OrderbookCWAPIClient

	atomicBool atomic.Bool

	orderMapByPoolID sync.Map

	keyring           keyring.Keyring
	defaultQuoteDenom string

	logger log.Logger
}

var _ domain.EndBlockProcessPlugin = &orderbookFillerIngestPlugin{}

const (
	baseDenom = "uosmo"
)

var (
	minBalanceValueInUSDC = osmomath.NewInt(10)
)

func New(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, tokensUseCase mvc.TokensUsecase, passthroughGRPCClient passthroughdomain.PassthroughGRPCClient, keyring keyring.Keyring, defaultQuoteDenom string, logger log.Logger) *orderbookFillerIngestPlugin {
	return &orderbookFillerIngestPlugin{
		poolsUseCase:  poolsUseCase,
		routerUseCase: routerUseCase,
		tokensUseCase: tokensUseCase,

		passthroughGRPCClient: passthroughGRPCClient,
		// TODO: set
		// orderbookCWAAPIClient: orderBookCWAPIClient,

		atomicBool: atomic.Bool{},

		orderMapByPoolID: sync.Map{},

		keyring:           keyring,
		defaultQuoteDenom: defaultQuoteDenom,

		logger: logger,
	}
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin.
func (o *orderbookFillerIngestPlugin) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	o.logger.Info("Processing end block", zap.Uint64("blockHeight", blockHeight))

	return nil
}
