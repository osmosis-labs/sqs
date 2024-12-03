package basefee

import (
	"context"

	txfeestypes "github.com/osmosis-labs/osmosis/v28/x/txfees/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/log"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"go.uber.org/zap"
)

type baseFeeEndBlockUpdatePlugin struct {
	routerRepository routerrepo.RouterRepository
	txfeesClient     txfeestypes.QueryClient
	logger           log.Logger
}

func NewEndBlockUpdatePlugin(routerRepository routerrepo.RouterRepository, txfeesClient txfeestypes.QueryClient, logger log.Logger) *baseFeeEndBlockUpdatePlugin {
	return &baseFeeEndBlockUpdatePlugin{
		routerRepository: routerRepository,
		txfeesClient:     txfeesClient,
		logger:           logger,
	}
}

// ProcessEndBlock calculates the base fee for the current block and updates the router repository with the new base fee.
func (p *baseFeeEndBlockUpdatePlugin) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	baseFee, err := tx.CalculateFeePrice(ctx, p.txfeesClient)
	if err != nil {
		p.logger.Error("failed to calculate fee price", zap.Error(err))
	}

	p.routerRepository.SetBaseFee(baseFee)

	return nil
}

var _ domain.EndBlockProcessPlugin = &baseFeeEndBlockUpdatePlugin{}
