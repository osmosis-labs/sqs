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
	poolsWorkers "github.com/osmosis-labs/sqs/pools/usecase/workers"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
	pricingWorker "github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"

	"github.com/osmosis-labs/sqs/sqsdomain/json"
	"github.com/osmosis-labs/sqs/sqsdomain/proto/types"

	chaininforepo "github.com/osmosis-labs/sqs/chaininfo/repository"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
)

type ingestUseCase struct {
	codec codec.Codec

	routerRepository    routerrepo.RouterRepository
	poolsUseCase        mvc.PoolsUsecase
	routerUsecase       mvc.RouterUsecase
	tokensUseCase       mvc.TokensUsecase
	chainInfoRepository chaininforepo.ChainInfoRepository

	usdcPriceUpdateWorker     pricingWorker.PricingWorker
	poolLiquidityUpdateWorker poolsWorkers.PoolLiquidityComputeWorker

	pricingStrategy domain.PricingStrategy
	tvlQuoteDenom   string

	startProcessingTime time.Time

	isColdStart bool

	logger log.Logger
}

var (
	_ mvc.IngestUsecase = &ingestUseCase{}
)

// NewIngestUsecase will create a new pools use case object
func NewIngestUsecase(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, chainInfoRepository chaininforepo.ChainInfoRepository, routerRepository routerrepo.RouterRepository, tokensUseCase mvc.TokensUsecase, codec codec.Codec, pricingConfig domain.PricingConfig, logger log.Logger) (mvc.IngestUsecase, error) {
	tvlQuoteDenom, err := tokensUseCase.GetChainDenom(context.Background(), pricingConfig.DefaultQuoteHumanDenom)
	if err != nil {
		return nil, err
	}

	// TODO: lift it up and reuse with the tokens handler
	pricingConfig.MinOSMOLiquidity = 0
	pricingStrategy, err := pricing.NewPricingStrategy(pricingConfig, tokensUseCase, routerUseCase)
	if err != nil {
		return nil, err
	}

	poolLiquidityComputeWorker := poolsWorkers.New(tokensUseCase, poolsUseCase)

	pricingUpdateListeners := []pricingWorker.PricingUpdateListener{
		poolLiquidityComputeWorker,
	}

	pricingStrategies := map[domain.PricingSource]domain.PricingStrategy{
		domain.ChainPricingSource: pricingStrategy,
	}

	return &ingestUseCase{
		codec: codec,

		routerRepository:    routerRepository,
		chainInfoRepository: chainInfoRepository,
		routerUsecase:       routerUseCase,
		poolsUseCase:        poolsUseCase,

		pricingStrategy: pricingStrategy,
		tvlQuoteDenom:   tvlQuoteDenom,

		logger: logger,

		isColdStart: true,

		tokensUseCase: tokensUseCase,

		usdcPriceUpdateWorker:     pricingWorker.New(pricingStrategies, tvlQuoteDenom, pricingUpdateListeners),
		poolLiquidityUpdateWorker: poolLiquidityComputeWorker,
	}, nil
}

type poolResult struct {
	pool sqsdomain.PoolI
	err  error
}

func (p *ingestUseCase) parsePoolData(ctx context.Context, poolData []*types.PoolData) ([]sqsdomain.PoolI, map[string]struct{}, error) {
	poolResultChan := make(chan poolResult, len(poolData))

	for _, pool := range poolData {
		go func(pool *types.PoolData) {
			poolResultData, err := p.processPool(pool)

			poolResultChan <- poolResult{
				pool: poolResultData,
				err:  err,
			}
		}(pool)
	}

	parsedPools := make([]sqsdomain.PoolI, 0, len(poolData))
	denomsToPoolIDsMap := make(map[string]struct{})

	for i := 0; i < len(poolData); i++ {
		select {
		case poolResult := <-poolResultChan:
			if poolResult.err != nil {
				// TODO: log and/or telemetry
				continue
			}

			// Update unique denoms
			for _, balance := range poolResult.pool.GetSQSPoolModel().Balances {
				denomsToPoolIDsMap[balance.Denom] = struct{}{}
			}

			parsedPools = append(parsedPools, poolResult.pool)
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}

	return parsedPools, denomsToPoolIDsMap, nil
}

func (p *ingestUseCase) ProcessBlockData(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap, poolData []*types.PoolData) (err error) {
	p.logger.Info("starting block processing", zap.Uint64("height", height))

	p.startProcessingTime = time.Now()

	p.routerRepository.SetTakerFees(ctx, takerFeesMap)

	// Parse the pools
	pools, uniqueDenoms, err := p.parsePoolData(ctx, poolData)
	if err != nil {
		return err
	}

	// Store the pools
	if err := p.poolsUseCase.StorePools(pools); err != nil {
		return err
	}

	updatedPoolIDs := make([]uint64, 0, len(pools))
	for _, pool := range pools {
		updatedPoolIDs = append(updatedPoolIDs, pool.GetId())
	}

	// Get all pools (already updated with the newly ingested pools)
	allPools, err := p.poolsUseCase.GetAllPools(ctx)
	if err != nil {
		return err
	}

	p.logger.Info("sorting pools", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(p.startProcessingTime)))

	// Sort the pools and store them in the router.
	if err := p.routerUsecase.SortPools(ctx, allPools); err != nil {
		return err
	}

	p.logger.Info("queuing pool liquidity updates & computing prices", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(p.startProcessingTime)))

	// Note: we must queue the update before we start updating prices as pool liquidity
	// worker listens for the pricing updates at the same height.
	p.poolLiquidityUpdateWorker.QueuePoolLiquidityCompute(ctx, height, updatedPoolIDs)
	p.usdcPriceUpdateWorker.UpdatePricesAsync(height, uniqueDenoms)

	// Store the latest ingested height.
	p.chainInfoRepository.StoreLatestHeight(height)

	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(p.startProcessingTime)))

	return nil
}

// processPool processes the pool data and returns the pool object
// For concentrated pools, it also processes the tick model
func (p *ingestUseCase) processPool(pool *types.PoolData) (sqsdomain.PoolI, error) {
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
