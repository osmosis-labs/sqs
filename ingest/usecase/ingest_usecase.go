package usecase

import (
	"context"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"go.uber.org/zap"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v23/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/sqsdomain/json"
	"github.com/osmosis-labs/sqs/sqsdomain/proto/types"
)

type ingestUseCase struct {
	codec codec.Codec

	poolsUseCase     mvc.PoolsUsecase
	routerUsecase    mvc.RouterUsecase
	tokensUseCase    mvc.TokensUsecase
	chainInfoUseCase mvc.ChainInfoUsecase

	logger log.Logger
}

type poolResult struct {
	pool sqsdomain.PoolI
	err  error
}

var (
	_ mvc.IngestUsecase = &ingestUseCase{}
)

// NewIngestUsecase will create a new pools use case object
func NewIngestUsecase(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, chainInfoUseCase mvc.ChainInfoUsecase, tokensUseCase mvc.TokensUsecase, codec codec.Codec, pricingConfig domain.PricingConfig, logger log.Logger) (mvc.IngestUsecase, error) {
	return &ingestUseCase{
		codec: codec,

		chainInfoUseCase: chainInfoUseCase,
		routerUsecase:    routerUseCase,
		poolsUseCase:     poolsUseCase,

		logger: logger,

		tokensUseCase: tokensUseCase,
	}, nil
}

func (p *ingestUseCase) ProcessBlockData(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap, poolData []*types.PoolData) (err error) {
	p.logger.Info("starting block processing", zap.Uint64("height", height))

	startProcessingTime := time.Now()

	p.routerUsecase.SetTakerFees(takerFeesMap)

	// Parse the pools
	pools, err := p.parsePoolData(ctx, poolData)
	if err != nil {
		return err
	}

	// Store the pools
	if err := p.poolsUseCase.StorePools(pools); err != nil {
		return err
	}

	// Get all pools (already updated with the newly ingested pools)
	allPools, err := p.poolsUseCase.GetAllPools()
	if err != nil {
		return err
	}

	p.logger.Info("sorting pools", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(startProcessingTime)))

	// Sort the pools and store them in the router.
	if err := p.routerUsecase.SortPools(ctx, allPools); err != nil {
		return err
	}
	// Store the latest ingested height.
	p.chainInfoUseCase.StoreLatestHeight(height)

	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(startProcessingTime)))

	return nil
}

// parsePoolData parses the pool data and returns the pool objects.
func (p *ingestUseCase) parsePoolData(ctx context.Context, poolData []*types.PoolData) ([]sqsdomain.PoolI, error) {
	poolResultChan := make(chan poolResult, len(poolData))

	// Parse the pools concurrently
	for _, pool := range poolData {
		go func(pool *types.PoolData) {
			poolResultData, err := p.parsePool(pool)

			poolResultChan <- poolResult{
				pool: poolResultData,
				err:  err,
			}
		}(pool)
	}

	parsedPools := make([]sqsdomain.PoolI, 0, len(poolData))

	// Collect the parsed pools
	for i := 0; i < len(poolData); i++ {
		select {
		case poolResult := <-poolResultChan:
			if poolResult.err != nil {
				// TODO: log and/or telemetry
				continue
			}

			parsedPools = append(parsedPools, poolResult.pool)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return parsedPools, nil
}

// parsePool parses the pool data and returns the pool object
// For concentrated pools, it also processes the tick model
func (p *ingestUseCase) parsePool(pool *types.PoolData) (sqsdomain.PoolI, error) {
	poolWrapper := sqsdomain.PoolWrapper{}

	if err := p.codec.UnmarshalInterfaceJSON(pool.ChainModel, &poolWrapper.ChainModel); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(pool.SqsModel, &poolWrapper.SQSModel); err != nil {
		return nil, err
	}

	if poolWrapper.GetType() == poolmanagertypes.Concentrated {
		poolWrapper.TickModel = &sqsdomain.TickModel{}
		if err := json.Unmarshal(pool.TickModel, poolWrapper.TickModel); err != nil {
			return nil, err
		}
	}

	return &poolWrapper, nil
}
