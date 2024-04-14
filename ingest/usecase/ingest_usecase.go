package usecase

import (
	"context"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"go.uber.org/zap"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v24/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
	pricingWorker "github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"

	"github.com/osmosis-labs/sqs/sqsdomain/json"
	"github.com/osmosis-labs/sqs/sqsdomain/proto/types"
)

type ingestUseCase struct {
	codec codec.Codec

	poolsUseCase     mvc.PoolsUsecase
	routerUsecase    mvc.RouterUsecase
	tokensUseCase    mvc.TokensUsecase
	chainInfoUseCase mvc.ChainInfoUsecase

	usdcPriceUpdateWorker pricingWorker.PricingWorker

	pricingStrategy domain.PricingSource
	tvlQuoteDenom   string

	startProcessingTime time.Time

	isColdStart bool

	logger log.Logger
}

var (
	_ mvc.IngestUsecase = &ingestUseCase{}
)

// NewIngestUsecase will create a new pools use case object
func NewIngestUsecase(poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, chainInfoUseCase mvc.ChainInfoUsecase, tokensUseCase mvc.TokensUsecase, codec codec.Codec, pricingConfig domain.PricingConfig, logger log.Logger) (mvc.IngestUsecase, error) {
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

	// TODO: add healthcheck and tokens usecase
	pricingUpdateListeners := []pricingWorker.PricingUpdateListener{}

	return &ingestUseCase{
		codec: codec,

		routerUsecase:    routerUseCase,
		chainInfoUseCase: chainInfoUseCase,
		poolsUseCase:     poolsUseCase,

		pricingStrategy: pricingStrategy,
		tvlQuoteDenom:   tvlQuoteDenom,

		logger: logger,

		isColdStart: true,

		tokensUseCase: tokensUseCase,

		usdcPriceUpdateWorker: pricingWorker.New(tokensUseCase, tvlQuoteDenom, pricingUpdateListeners, logger),
	}, nil
}

type poolResult struct {
	pool sqsdomain.PoolI
	err  error
}

func (p *ingestUseCase) ProcessBlockData(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap, poolData []*types.PoolData) (err error) {
	p.logger.Info("starting block processing", zap.Uint64("height", height))

	startProcessingTime := time.Now()

	p.routerUsecase.SetTakerFees(takerFeesMap)

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
	allPools, err := p.poolsUseCase.GetAllPools()
	if err != nil {
		return err
	}

	p.logger.Info("sorting pools", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(startProcessingTime)))

	// Sort the pools and store them in the router.
	if err := p.routerUsecase.SortPools(ctx, allPools); err != nil {
		return err
	}

	// Note: we must queue the update before we start updating prices as pool liquidity
	// worker listens for the pricing updates at the same height.
	p.usdcPriceUpdateWorker.UpdatePricesAsync(height, uniqueDenoms)

	// Store the latest ingested height.
	p.chainInfoUseCase.StoreLatestHeight(height)

	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration_since_start", time.Since(p.startProcessingTime)))

	// Observe the processing duration with height
	domain.SQSIngestHandlerProcessBlockDurationHistogram.WithLabelValues(strconv.FormatUint(height, 10)).Observe(float64(time.Since(startProcessingTime).Nanoseconds()))

	return nil
}

func (p *ingestUseCase) parsePoolData(ctx context.Context, poolData []*types.PoolData) ([]sqsdomain.PoolI, map[string]struct{}, error) {
	poolResultChan := make(chan poolResult, len(poolData))

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
	denomsMap := make(map[string]struct{})

	for i := 0; i < len(poolData); i++ {
		select {
		case poolResult := <-poolResultChan:
			if poolResult.err != nil {
				// TODO: log and/or telemetry
				continue
			}

			// Update unique denoms
			for _, balance := range poolResult.pool.GetSQSPoolModel().Balances {
				denomsMap[balance.Denom] = struct{}{}
			}

			parsedPools = append(parsedPools, poolResult.pool)
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}

	return parsedPools, denomsMap, nil
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
