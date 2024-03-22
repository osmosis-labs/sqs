package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v23/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"

	"github.com/osmosis-labs/sqs/sqsdomain/json"
	"github.com/osmosis-labs/sqs/sqsdomain/proto/types"

	"github.com/osmosis-labs/sqs/sqsdomain/repository"
	chaininforepo "github.com/osmosis-labs/sqs/sqsdomain/repository/memory/chaininfo"
	routerredisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis/router"
)

type ingestUseCase struct {
	redisRepositoryManager repository.TxManager
	codec                  codec.Codec

	pricingStrategy domain.PricingStrategy
	tvlQuoteDenom   string

	routerRepository    routerredisrepo.RouterRepository
	poolsUseCase        mvc.PoolsUsecase
	routerUsecase       mvc.RouterUsecase
	tokensUseCase       mvc.TokensUsecase
	chainInfoRepository chaininforepo.ChainInfoRepository
	pools               []sqsdomain.PoolI

	startProcessingTime time.Time

	logger log.Logger
}

var _ mvc.IngestUsecase = &ingestUseCase{}

// NewIngestUsecase will create a new pools use case object
func NewIngestUsecase(redisRepositoryManager repository.TxManager, poolsUseCase mvc.PoolsUsecase, routerUseCase mvc.RouterUsecase, chainInfoRepository chaininforepo.ChainInfoRepository, routerRepository routerredisrepo.RouterRepository, tokensUseCase mvc.TokensUsecase, codec codec.Codec, pricingConfig domain.PricingConfig, logger log.Logger) (mvc.IngestUsecase, error) {
	tvlQuoteDenom, err := tokensUseCase.GetChainDenom(context.Background(), pricingConfig.DefaultQuoteHumanDenom)
	if err != nil {
		return nil, err
	}

	// TODO: lift it up and reuse with the tokens handler
	pricingStrategy, err := pricing.NewPricingStrategy(pricingConfig, tokensUseCase, routerUseCase)
	if err != nil {
		return nil, err
	}

	return &ingestUseCase{
		// TODO: change this
		redisRepositoryManager: redisRepositoryManager,
		codec:                  codec,

		routerRepository:    routerRepository,
		chainInfoRepository: chainInfoRepository,
		routerUsecase:       routerUseCase,
		poolsUseCase:        poolsUseCase,

		pricingStrategy: pricingStrategy,
		tvlQuoteDenom:   tvlQuoteDenom,

		pools: make([]sqsdomain.PoolI, 0),

		logger: logger,

		tokensUseCase: tokensUseCase,
	}, nil
}

type poolResult struct {
	pool sqsdomain.PoolI
	err  error
}

// ProcessPoolChunk implements mvc.IngestUsecase.
func (p *ingestUseCase) ProcessPoolChunk(ctx context.Context, poolData []*types.PoolData) error {
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

	for i := 0; i < len(poolData); i++ {
		select {
		case poolResult := <-poolResultChan:
			if poolResult.err != nil {
				// TODO: log and/or telemetry
				continue
			}

			parsedPools = append(parsedPools, poolResult.pool)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	p.pools = append(p.pools, parsedPools...)

	return nil
}

func (p *ingestUseCase) StartBlockProcess(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap) (err error) {
	p.logger.Info("starting block processing", zap.Uint64("height", height))

	p.startProcessingTime = time.Now()

	p.pools = make([]sqsdomain.PoolI, 0)

	currentTx := p.redisRepositoryManager.StartTx()

	// TODO: revisit the way we store taker fees in the future.
	for denomPair, takerFee := range takerFeesMap {
		if err := p.routerRepository.SetTakerFee(ctx, currentTx, denomPair.Denom0, denomPair.Denom1, takerFee); err != nil {
			return err
		}
	}

	if err := currentTx.Exec(ctx); err != nil {
		return err
	}

	return nil
}

func (p *ingestUseCase) EndBlockProcess(ctx context.Context, height uint64) (err error) {
	if err := p.poolsUseCase.StorePools(p.pools); err != nil {
		return err
	}

	if err := p.routerUsecase.SortPools(ctx, p.pools); err != nil {
		return err
	}

	// Store the latest ingested height.
	p.chainInfoRepository.StoreLatestHeight(height)

	p.pools = make([]sqsdomain.PoolI, 0)

	p.logger.Info("completed block processing", zap.Uint64("height", height), zap.Duration("duration", time.Since(p.startProcessingTime)))

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

// nolint: unused
func (p *ingestUseCase) computeBalanceTVL(ctx context.Context, poolBalances sdk.Coins) (osmomath.Int, string) {
	usdcTVL := osmomath.ZeroDec()

	tvlError := ""

	for _, balance := range poolBalances {
		// Skip gamm shares
		if strings.Contains(balance.Denom, "gamm/pool") {
			continue
		}

		currentTokenTVL, err := p.computeCoinTVL(ctx, balance)
		if err != nil {
			// Silenly skip.
			// Append tvl error but continue to the next token
			tvlError += fmt.Sprintf(" %s", err)
			continue
		}

		usdcTVL = usdcTVL.AddMut(currentTokenTVL)
	}

	return usdcTVL.TruncateInt(), tvlError
}

// computeCoinTVl computes the equivalent of the given coin in the desired quote denom that is set on ingester
// nolint: unused
func (p *ingestUseCase) computeCoinTVL(ctx context.Context, coin sdk.Coin) (osmomath.Dec, error) {
	// Get price. Note that this price is descaled from on-chain precision.
	price, err := p.pricingStrategy.GetPrice(ctx, coin.Denom, p.tvlQuoteDenom)
	if err != nil {
		return osmomath.Dec{}, err
	}

	// Get on-chain scaling factor for the coin denom.
	baseScalingFactor, err := p.tokensUseCase.GetChainScalingFactorByDenomMut(ctx, coin.Denom)
	if err != nil {
		return osmomath.Dec{}, err
	}

	currentTokenTVL := osmomath.NewBigDecFromBigInt(coin.Amount.BigIntMut()).MulMut(price)
	isOriginalAmountZero := coin.Amount.IsZero()

	// Truncation in intermediary operation - return error.
	if currentTokenTVL.IsZero() && !isOriginalAmountZero {
		return osmomath.Dec{}, fmt.Errorf("truncation occurred when multiplying coin (%s) by %s price %s", coin, p.tvlQuoteDenom, price)
	}

	// Truncation in intermediary operation - return error.
	currentTokenTVL = currentTokenTVL.QuoMut(osmomath.BigDecFromDec(baseScalingFactor))
	if currentTokenTVL.IsZero() && !isOriginalAmountZero {
		return osmomath.Dec{}, fmt.Errorf("truncation occurred when multiplying (%s) of denom (%s) by the scaling factor (%s)", currentTokenTVL, coin.Denom, baseScalingFactor)
	}

	return currentTokenTVL.Dec(), nil
}
