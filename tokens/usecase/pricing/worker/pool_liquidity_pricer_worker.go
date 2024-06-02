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
	poolsUseCase  mvc.PoolsUsecase
	tokensUseCase mvc.TokensUsecase

	liquidityPricer domain.LiquidityPricer

	// Denom -> Last height of the pricing update.
	// This exists because pricing computations are asynchronous. As a result, a pricing update for a later
	// height might arrive before a pricing update for an earlier height. This map is used to ensure that
	// the latest height pricing update for a denom is used.
	latestHeightForDenom sync.Map
}

func NewPoolLiquidityWorker(tokensUseCase mvc.TokensUsecase, poolsUseCase mvc.PoolsUsecase, liquidityPricer domain.LiquidityPricer) domain.PricingUpdateListener {
	return &poolLiquidityPricerWorker{

		poolsUseCase:  poolsUseCase,
		tokensUseCase: tokensUseCase,

		liquidityPricer: liquidityPricer,

		latestHeightForDenom: sync.Map{},
	}
}

// OnPricingUpdate implements worker.PricingUpdateListener.
func (p *poolLiquidityPricerWorker) OnPricingUpdate(ctx context.Context, height int64, blockPoolMetadata domain.BlockPoolMetadata, baseDenomPriceUpdates domain.PricesResult, quoteDenom string) error {
	// Note: in the future, if we add pool liquidity pricing, we can process the computation in separate goroutines
	// for concurrency.
	repricedTokenMetadata := p.repriceDenomMetadata(height, baseDenomPriceUpdates, quoteDenom, blockPoolMetadata.DenomLiquidityMap)

	// Update the pool denom metadata.
	p.tokensUseCase.UpdatePoolDenomMetadata(repricedTokenMetadata)

	return nil
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

func (p *poolLiquidityPricerWorker) repriceDenomMetadata(updateHeight int64, blockPriceUpdates domain.PricesResult, quoteDenom string, blockDenomLiquidityUpdatesMap domain.DenomLiquidityMap) domain.PoolDenomMetaDataMap {
	blockTokenMetadataUpdates := make(domain.PoolDenomMetaDataMap, len(blockDenomLiquidityUpdatesMap))

	// Iterate over the denoms updated within the block
	for updatedBlockDenom, blockPoolDenomLiquidityData := range blockDenomLiquidityUpdatesMap {
		// Skip if the denom has a later update than the current height.
		if p.hasLaterUpdateThanHeight(updatedBlockDenom, uint64(updateHeight)) {
			continue
		}

		quotePrices, ok := blockPriceUpdates[updatedBlockDenom]
		if !ok {
			// If no price is available, keep the total liquidity but set the capitalization to zero.
			// Should not happen in practice as we reprice all denoms in the block.
			blockTokenMetadataUpdates.Set(updatedBlockDenom, blockPoolDenomLiquidityData.TotalLiquidity, osmomath.ZeroInt())
			continue
		}

		// Store the height for the denom.
		p.storeHeightForDenom(updatedBlockDenom, uint64(updateHeight))

		currentPrice, ok := quotePrices[quoteDenom]
		if !ok {
			// If no price is available, keep the total liquidity but set the capitalization to zero.
			blockTokenMetadataUpdates.Set(updatedBlockDenom, blockPoolDenomLiquidityData.TotalLiquidity, osmomath.ZeroInt())
		}

		// Get the scaling factor for the base denom.
		currentBaseScalingFactor, err := p.tokensUseCase.GetChainScalingFactorByDenomMut(updatedBlockDenom)
		if err != nil {
			// If there is an error, silently ignore and skip it.
			continue
		}

		currentPriceInfo := domain.DenomPriceInfo{
			Price:         currentPrice,
			ScalingFactor: currentBaseScalingFactor,
		}

		liquidityCapitalization, err := p.liquidityPricer.ComputeCoinCap(sdk.NewCoin(updatedBlockDenom, blockPoolDenomLiquidityData.TotalLiquidity), currentPriceInfo)
		if err != nil {
			// If there is an error, keep the total liquidity but set the capitalization to zero.
			blockTokenMetadataUpdates.Set(updatedBlockDenom, blockPoolDenomLiquidityData.TotalLiquidity, osmomath.ZeroInt())
		} else {
			// Set the computed liquidity liquidity capitalization.
			blockTokenMetadataUpdates.Set(updatedBlockDenom, blockPoolDenomLiquidityData.TotalLiquidity, liquidityCapitalization.TruncateInt())
		}
	}

	// Return the updated token metadata for testability
	return blockTokenMetadataUpdates
}

// storeHeightForDenom stores the latest height for the given denom.
func (p *poolLiquidityPricerWorker) storeHeightForDenom(denom string, height uint64) {
	p.latestHeightForDenom.Store(denom, height)
}
