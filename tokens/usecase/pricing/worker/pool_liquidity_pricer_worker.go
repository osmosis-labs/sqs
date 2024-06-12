package worker

import (
	"context"
	"strings"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

var (
	_ domain.PricingUpdateListener = &poolLiquidityPricerWorker{}
)

type poolLiquidityPricerWorker struct {
	tokenPoolLiquidityHandler mvc.TokensPoolLiquidityHandler
	poolHandler               PoolHandler

	updateListeners []domain.PoolLiquidityComputeListener

	liquidityPricer domain.LiquidityPricer

	// Denom -> Last height of the pricing update.
	// This exists because pricing computations are asynchronous. As a result, a pricing update for a later
	// height might arrive before a pricing update for an earlier height. This map is used to ensure that
	// the latest height pricing update for a denom is used.
	latestHeightForDenom sync.Map
}

type PoolHandler interface {
	// GetPools returns the pools corresponding to the given IDs.
	GetPools(poolIDs []uint64) ([]sqsdomain.PoolI, error)

	// StorePools stores the given pools in the usecase
	StorePools(pools []sqsdomain.PoolI) error
}

func NewPoolLiquidityWorker(tokensPoolLiquidityHandler mvc.TokensPoolLiquidityHandler, poolHandler PoolHandler, liquidityPricer domain.LiquidityPricer) domain.PoolLiquidityPricerWorker {
	return &poolLiquidityPricerWorker{
		tokenPoolLiquidityHandler: tokensPoolLiquidityHandler,
		poolHandler:               poolHandler,

		updateListeners: []domain.PoolLiquidityComputeListener{},

		liquidityPricer: liquidityPricer,

		latestHeightForDenom: sync.Map{},
	}
}

// OnPricingUpdate implements worker.PricingUpdateListener.
func (p *poolLiquidityPricerWorker) OnPricingUpdate(ctx context.Context, height uint64, blockPoolMetadata domain.BlockPoolMetadata, baseDenomPriceUpdates domain.PricesResult, quoteDenom string) error {
	start := time.Now()

	// wg := sync.WaitGroup{}

	// wg.Add(1)
	// go func() {
	// 	defer wg.Done()
	// 	// Note: in the future, if we add pool liquidity pricing, we can process the computation in separate goroutines
	// 	// for concurrency.
	// 	repricedTokenMetadata := p.RepriceDenomMetadata(height, baseDenomPriceUpdates, quoteDenom, blockPoolMetadata.DenomPoolLiquidityMap)

	// 	// Update the pool denom metadata.
	// 	p.tokenPoolLiquidityHandler.UpdatePoolDenomMetadata(repricedTokenMetadata)
	// }()
	// Note: in the future, if we add pool liquidity pricing, we can process the computation in separate goroutines
	// for concurrency.
	repricedTokenMetadata := p.RepriceDenomMetadata(height, baseDenomPriceUpdates, quoteDenom, blockPoolMetadata)

	// Update the pool denom metadata.
	p.tokenPoolLiquidityHandler.UpdatePoolDenomMetadata(repricedTokenMetadata)

	// wg.Add(1)
	// go func() {
	// 	defer wg.Done()

	// 	p.repricePoolLiquidityCap(blockPoolMetadata.PoolIDs, baseDenomPriceUpdates, quoteDenom)
	// }()

	p.repricePoolLiquidityCap(blockPoolMetadata.PoolIDs, baseDenomPriceUpdates, quoteDenom)

	// Wait for goroutines to finish processing.
	// wg.Wait()

	// Notify listeners.
	for _, listener := range p.updateListeners {
		// Avoid checking error since we want to execute all listeners.
		_ = listener.OnPoolLiquidityCompute(int64(height))
	}

	// Measure duration
	domain.SQSPoolLiquidityPricingWorkerComputeDurationGauge.Add(float64(time.Since(start).Milliseconds()))

	return nil
}

// RepriceDenomMetadata implements domain.PoolLiquidityPricerWorker
func (p *poolLiquidityPricerWorker) RepriceDenomMetadata(updateHeight uint64, blockPriceUpdates domain.PricesResult, quoteDenom string, blockPoolMetadata domain.BlockPoolMetadata) domain.PoolDenomMetaDataMap {
	blockTokenMetadataUpdates := make(domain.PoolDenomMetaDataMap, len(blockPoolMetadata.UpdatedDenoms))

	// Iterate over the denoms updated within the block
	for updatedBlockDenom := range blockPoolMetadata.UpdatedDenoms {
		if strings.Contains(updatedBlockDenom, "gamm/pool") {
			continue
		}

		blockPoolDenomLiquidityData, ok := blockPoolMetadata.DenomPoolLiquidityMap[updatedBlockDenom]
		if !ok {
			// TODO: error
			continue
		}

		// Skip if the denom has a later update than the current height.
		if p.hasLaterUpdateThanHeight(updatedBlockDenom, updateHeight) {
			continue
		}

		totalLiquidityForDenom := blockPoolDenomLiquidityData.TotalLiquidity

		price := blockPriceUpdates.GetPriceForDenom(updatedBlockDenom, quoteDenom)

		if price.IsZero() {
			continue
		}

		liquidityCapitalization := p.liquidityPricer.PriceCoin(sdk.NewCoin(updatedBlockDenom, totalLiquidityForDenom), price)

		blockTokenMetadataUpdates.Set(updatedBlockDenom, totalLiquidityForDenom, liquidityCapitalization, price)

		// Store the height for the denom.
		p.StoreHeightForDenom(updatedBlockDenom, updateHeight)
	}

	// Return the updated token metadata for testability
	return blockTokenMetadataUpdates
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
func (p *poolLiquidityPricerWorker) repricePoolLiquidityCap(poolIDs map[uint64]struct{}, blockPriceUpdates domain.PricesResult, quoteDenom string) error {
	blockPoolIDs := domain.KeysFromMap(poolIDs)

	pools, err := p.poolHandler.GetPools(blockPoolIDs)
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
