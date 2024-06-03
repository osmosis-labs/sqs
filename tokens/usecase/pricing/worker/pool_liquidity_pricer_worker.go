package worker

import (
	"context"
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

var (
	_ domain.PricingUpdateListener = &poolLiquidityPricerWorker{}
)

type poolLiquidityPricerWorker struct {
	tokenPoolLiquidityHandler mvc.TokensPoolLiquidityHandler

	liquidityPricer domain.LiquidityPricer

	// Denom -> Last height of the pricing update.
	// This exists because pricing computations are asynchronous. As a result, a pricing update for a later
	// height might arrive before a pricing update for an earlier height. This map is used to ensure that
	// the latest height pricing update for a denom is used.
	latestHeightForDenom sync.Map
}

func NewPoolLiquidityWorker(tokensPoolLiquidityHandler mvc.TokensPoolLiquidityHandler, liquidityPricer domain.LiquidityPricer) domain.PoolLiquidityPricerWorker {
	return &poolLiquidityPricerWorker{
		tokenPoolLiquidityHandler: tokensPoolLiquidityHandler,

		liquidityPricer: liquidityPricer,

		latestHeightForDenom: sync.Map{},
	}
}

// OnPricingUpdate implements worker.PricingUpdateListener.
func (p *poolLiquidityPricerWorker) OnPricingUpdate(ctx context.Context, height uint64, blockPoolMetadata domain.BlockPoolMetadata, baseDenomPriceUpdates domain.PricesResult, quoteDenom string) error {
	// Note: in the future, if we add pool liquidity pricing, we can process the computation in separate goroutines
	// for concurrency.
	repricedTokenMetadata := p.RepriceDenomMetadata(height, baseDenomPriceUpdates, quoteDenom, blockPoolMetadata.DenomLiquidityMap)

	// Update the pool denom metadata.
	p.tokenPoolLiquidityHandler.UpdatePoolDenomMetadata(repricedTokenMetadata)

	return nil
}

// RepriceDenomMetadata implements domain.PoolLiquidityPricerWorker
func (p *poolLiquidityPricerWorker) RepriceDenomMetadata(updateHeight uint64, blockPriceUpdates domain.PricesResult, quoteDenom string, blockDenomLiquidityUpdatesMap domain.DenomLiquidityMap) domain.PoolDenomMetaDataMap {
	blockTokenMetadataUpdates := make(domain.PoolDenomMetaDataMap, len(blockDenomLiquidityUpdatesMap))

	// Iterate over the denoms updated within the block
	for updatedBlockDenom, blockPoolDenomLiquidityData := range blockDenomLiquidityUpdatesMap {
		// Skip if the denom has a later update than the current height.
		if p.hasLaterUpdateThanHeight(updatedBlockDenom, updateHeight) {
			continue
		}

		totalLiquidityForDenom := blockPoolDenomLiquidityData.TotalLiquidity

		price := blockPriceUpdates.GetPriceForDenom(updatedBlockDenom, quoteDenom)

		liquidityCapitalization := p.ComputeLiquidityCapitalization(updatedBlockDenom, totalLiquidityForDenom, price)

		blockTokenMetadataUpdates.Set(updatedBlockDenom, totalLiquidityForDenom, liquidityCapitalization, price)

		// Store the height for the denom.
		p.StoreHeightForDenom(updatedBlockDenom, updateHeight)
	}

	// Return the updated token metadata for testability
	return blockTokenMetadataUpdates
}

// ComputeLiquidityCapitalization implements domain.PoolLiquidityPricerWorker.
func (p *poolLiquidityPricerWorker) ComputeLiquidityCapitalization(denom string, totalLiquidity osmomath.Int, price osmomath.BigDec) osmomath.Int {
	if price.IsZero() {
		// If the price is zero, set the capitalization to zero.
		return osmomath.ZeroInt()
	}

	// Get the scaling factor for the base denom.
	baseScalingFactor, err := p.tokenPoolLiquidityHandler.GetChainScalingFactorByDenomMut(denom)
	if err != nil {
		// If there is an error, keep the total liquidity but set the capitalization to zero.
		return osmomath.ZeroInt()
	}

	priceInfo := domain.DenomPriceInfo{
		Price:         price,
		ScalingFactor: baseScalingFactor,
	}

	liquidityCapitalization, err := p.liquidityPricer.ComputeCoinCap(sdk.NewCoin(denom, totalLiquidity), priceInfo)
	if err != nil {
		// If there is an error, keep the total liquidity but set the capitalization to zero.
		return osmomath.ZeroInt()
	}

	return liquidityCapitalization.TruncateInt()
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
