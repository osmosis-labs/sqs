package worker

import (
	"context"
	"strings"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	stdosmomath "github.com/osmosis-labs/sqs/domain/osmomath"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

const (
	gammSharePrefix = "gamm/pool"
)

var (
	_ domain.PricingUpdateListener     = &poolLiquidityPricerWorker{}
	_ domain.PoolLiquidityPricerWorker = &poolLiquidityPricerWorker{}
)

type poolLiquidityPricerWorker struct {
	tokenPoolLiquidityHandler mvc.TokensPoolLiquidityHandler
	tokensUsecase             mvc.TokensUsecase
	poolHandler               mvc.PoolHandler

	updateListeners []domain.PoolLiquidityComputeListener

	liquidityPricer domain.LiquidityPricer

	logger log.Logger

	// Denom -> Last height of the pricing update.
	// This exists because pricing computations are asynchronous. As a result, a pricing update for a later
	// height might arrive before a pricing update for an earlier height. This map is used to ensure that
	// the latest height pricing update for a denom is used.
	latestHeightForDenom sync.Map
}

func NewPoolLiquidityWorker(
	tokensUsecase mvc.TokensUsecase,
	tokensPoolLiquidityHandler mvc.TokensPoolLiquidityHandler,
	poolHandler mvc.PoolHandler,
	liquidityPricer domain.LiquidityPricer,
	logger log.Logger,
) *poolLiquidityPricerWorker {
	return &poolLiquidityPricerWorker{
		tokenPoolLiquidityHandler: tokensPoolLiquidityHandler,
		poolHandler:               poolHandler,
		tokensUsecase:             tokensUsecase,

		updateListeners: []domain.PoolLiquidityComputeListener{},

		liquidityPricer: liquidityPricer,

		logger: logger,

		latestHeightForDenom: sync.Map{},
	}
}

// OnPricingUpdate implements worker.PricingUpdateListener.
func (p *poolLiquidityPricerWorker) OnPricingUpdate(ctx context.Context, height uint64, blockPoolMetadata domain.BlockPoolMetadata, baseDenomPriceUpdates domain.PricesResult, quoteDenom string) (err error) {
	start := time.Now()

	defer func() {
		// Measure duration
		domain.SQSPoolLiquidityPricingWorkerComputeDurationGauge.Add(float64(time.Since(start).Milliseconds()))
	}()

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Note: in the future, if we add pool liquidity pricing, we can process the computation in separate goroutines
		// for concurrency.
		repricedTokenMetadata := p.RepriceDenomsMetadata(height, baseDenomPriceUpdates, quoteDenom, blockPoolMetadata)

		// Update the pool denom metadata.
		p.tokenPoolLiquidityHandler.UpdatePoolDenomMetadata(repricedTokenMetadata)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		// Note: the error is propagated to the caller because
		// the callee only errors on fatal issues that should invalidate health check.
		err = p.repricePoolLiquidityCap(blockPoolMetadata.PoolIDs, baseDenomPriceUpdates)
	}()

	// Wait for goroutines to finish processing.
	wg.Wait()

	// Notify listeners.
	for _, listener := range p.updateListeners {
		// Avoid checking error since we want to execute all listeners.
		_ = listener.OnPoolLiquidityCompute(ctx, height, blockPoolMetadata)
	}

	return nil
}

// RepriceDenomsMetadata implements domain.PoolLiquidityPricerWorker
func (p *poolLiquidityPricerWorker) RepriceDenomsMetadata(updateHeight uint64, blockPriceUpdates domain.PricesResult, quoteDenom string, blockPoolMetadata domain.BlockPoolMetadata) domain.PoolDenomMetaDataMap {
	blockTokenMetadataUpdates := make(domain.PoolDenomMetaDataMap, len(blockPoolMetadata.UpdatedDenoms))

	// Iterate over the denoms updated within the block
	for updatedBlockDenom := range blockPoolMetadata.UpdatedDenoms {
		if shouldSkipDenom := p.shouldSkipDenomRepricing(updatedBlockDenom, updateHeight); shouldSkipDenom {
			continue
		}

		poolDenomMetaData, err := p.CreatePoolDenomMetaData(updatedBlockDenom, updateHeight, blockPriceUpdates, quoteDenom, blockPoolMetadata)
		if err != nil {
			p.logger.Debug("error creating denom meta data", zap.Error(err))
		}

		blockTokenMetadataUpdates.Set(updatedBlockDenom, poolDenomMetaData)

		// Store the height for the denom.
		p.StoreHeightForDenom(updatedBlockDenom, updateHeight)
	}

	// Return the updated token metadata for testability
	return blockTokenMetadataUpdates
}

// CreatePoolDenomMetaData implements domain.PoolLiquidityPricerWorker
func (p *poolLiquidityPricerWorker) CreatePoolDenomMetaData(updatedBlockDenom string, updateHeight uint64, blockPriceUpdates domain.PricesResult, quoteDenom string, blockPoolMetadata domain.BlockPoolMetadata) (domain.PoolDenomMetaData, error) {
	price := blockPriceUpdates.GetPriceForDenom(updatedBlockDenom, quoteDenom)

	// Retrieve liquidity from block pool metadata.
	// Assummed zero if does not exist.
	totalLiquidityForDenom := osmomath.ZeroInt()
	blockPoolDenomLiquidityData, ok := blockPoolMetadata.DenomPoolLiquidityMap[updatedBlockDenom]
	if ok {
		totalLiquidityForDenom = blockPoolDenomLiquidityData.TotalLiquidity
	}

	liquidityCapitalization := p.liquidityPricer.PriceCoin(sdk.NewCoin(updatedBlockDenom, totalLiquidityForDenom), price)

	result := domain.PoolDenomMetaData{
		TotalLiquidity:    totalLiquidityForDenom,
		TotalLiquidityCap: liquidityCapitalization.TruncateInt(),
		Price:             price,
	}

	if !ok {
		return result, domain.DenomPoolLiquidityDataNotFoundError{
			Denom: updatedBlockDenom,
		}
	}

	if price.IsZero() {
		return result, domain.PriceNotFoundForPoolLiquidityCapError{
			Denom: updatedBlockDenom,
		}
	}

	totalEffectiveLiquidityCap, err := p.computeEffectiveLiquidityCap(blockPoolDenomLiquidityData, blockPriceUpdates, quoteDenom, updatedBlockDenom, price)
	if err != nil {
		return result, nil // on error we just skip the effective liquidity cap
	}

	result.TotalEffectiveLiquidityCap = totalEffectiveLiquidityCap.TruncateInt()

	return result, nil
}

// computeEffectiveLiquidityCap computes the effective liquidity cap for the given denom.
// Effective liquidity cap is the sum of the liquidity capitalization for each pool
// where the ratio of the minimum to maximum balance is greater than 0.01.
// If an error occurs preventing the calculation of the effective liquidity cap,
// the function returns the first error encountered.
func (p *poolLiquidityPricerWorker) computeEffectiveLiquidityCap(
	blockPoolDenomLiquidityData domain.DenomPoolLiquidityData,
	blockPriceUpdates domain.PricesResult,
	quoteDenom string,
	updatedBlockDenom string,
	price osmomath.BigDec,
) (osmomath.Dec, error) {
	var errors []error
	totalEffectiveLiquidityCap := osmomath.ZeroDec()
	for poolID, balance := range blockPoolDenomLiquidityData.Pools {
		pool, _, err := p.poolHandler.GetPools(domain.WithPoolIDFilter([]uint64{poolID}))
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if len(pool) == 0 {
			continue
		}

		var balances []osmomath.BigDec
		for _, coin := range pool[0].GetSQSPoolModel().Balances {
			metadata, err := p.tokensUsecase.GetMetadataByChainDenom(coin.Denom)
			if err != nil {
				errors = append(errors, err)
				continue
			}

			amount, err := osmomath.NewBigDecFromStr(coin.Amount.String())
			if err != nil {
				errors = append(errors, err)
				continue
			}

			price := blockPriceUpdates.GetPriceForDenom(coin.Denom, quoteDenom)

			normalized := amount.Quo(osmomath.NewBigDec(10).Power(osmomath.NewBigDec(int64(metadata.Precision)))).Mul(price)

			balances = append(balances, normalized)
		}

		min := stdosmomath.MinBigDec(balances...)
		max := stdosmomath.MaxBigDec(balances...)
		if max.IsZero() {
			continue // skip if max is zero
		}

		ratio := min.Quo(max)
		threshold := osmomath.NewBigDecWithPrec(1, 2) // 0.01
		if ratio.LTE(threshold) {
			continue // skip if ratio is less than 0.01
		}

		liquidityCapitalization := p.liquidityPricer.PriceCoin(sdk.NewCoin(updatedBlockDenom, balance), price)
		totalEffectiveLiquidityCap = totalEffectiveLiquidityCap.Add(liquidityCapitalization)
	}

	// If there are errors and the total effective liquidity cap is zero,
	// return the first error.
	if len(errors) > 0 && totalEffectiveLiquidityCap.IsZero() {
		return osmomath.ZeroDec(), errors[0]
	}

	return totalEffectiveLiquidityCap, nil
}

// shouldSkipDenomRepricing returns true if the denom repricing should be skipped.
// Specifically, if the denom is a gamm share denom or
// if the pool liquidity pricing worker already observed a later update
// than the given updateHeight.
func (p *poolLiquidityPricerWorker) shouldSkipDenomRepricing(denom string, updateHeight uint64) bool {
	return strings.Contains(denom, gammSharePrefix) || p.hasLaterUpdateThanHeight(denom, updateHeight)
}

// GetHeightForDenom implements domain.PoolLiquidityPricerWorker.
func (p *poolLiquidityPricerWorker) GetHeightForDenom(denom string) uint64 {
	heightObj, ok := p.latestHeightForDenom.Load(denom)
	if !ok {
		return 0
	}

	height, ok := heightObj.(uint64)
	if !ok {
		return 0
	}

	return height
}

// StoreHeightForDenom implements domain.PoolLiquidityPricerWorker.
func (p *poolLiquidityPricerWorker) StoreHeightForDenom(denom string, height uint64) {
	p.latestHeightForDenom.Store(denom, height)
}

// hasLaterUpdateThanHeight checks if the given denom has a later update than the current height.
// Returns true if the denom has a later update than the current height.
// False otherwise.
func (p *poolLiquidityPricerWorker) hasLaterUpdateThanHeight(denom string, height uint64) bool {
	latestHeightForDenomObj, ok := p.latestHeightForDenom.Load(denom)

	if ok {
		latestHeightForDenom, ok := latestHeightForDenomObj.(uint64)
		return ok && height < latestHeightForDenom
	}

	return false
}

// repricePoolLiquidityCap reprices pool liquidity capitalization for the given poolIDs, block price updates and quote denom.
// If fails to retrieve price for one of the denoms in balances, the liquidity capitalization for that denom would be zero.
func (p *poolLiquidityPricerWorker) repricePoolLiquidityCap(poolIDs map[uint64]struct{}, blockPriceUpdates domain.PricesResult) error {
	blockPoolIDs := domain.KeysFromMap(poolIDs)

	pools, _, err := p.poolHandler.GetPools(domain.WithPoolIDFilter(blockPoolIDs))
	if err != nil {
		return err
	}

	for i, pool := range pools {
		balances := pool.GetSQSPoolModel().Balances

		poolLiquidityCapitalization, poolLiquidityCapError := p.liquidityPricer.PriceBalances(balances, blockPriceUpdates)

		// Update the liquidity capitalization and error (if any)
		pools[i].SetLiquidityCap(poolLiquidityCapitalization)
		pools[i].SetLiquidityCapError(poolLiquidityCapError)
	}

	if err := p.poolHandler.StorePools(pools); err != nil {
		return err
	}

	return nil
}

// RegisterListener implements PoolLiquidityPricerWorker.
func (p *poolLiquidityPricerWorker) RegisterListener(listener domain.PoolLiquidityComputeListener) {
	p.updateListeners = append(p.updateListeners, listener)
}
