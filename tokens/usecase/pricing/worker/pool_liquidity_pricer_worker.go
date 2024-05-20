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

// hasLaterUpdateThanCurrent checks if the given denom has a later update than the current height.
// Returns true if the denom has a later update than the current height.
// False otherwise.
// TODO: test
func (p *poolLiquidityPricerWorker) hasLaterUpdateThanCurrent(denom string, currentHeight uint64) bool {
	latestHeightForDenomObj, ok := p.latestHeightForDenom.Load(denom)

	if ok {
		latestHeightForDenom, ok := latestHeightForDenomObj.(int64)
		return ok && int64(currentHeight) < latestHeightForDenom
	}

	return false
}

func (p *poolLiquidityPricerWorker) repriceDenomMetadata(updateHeight int64, blockPriceUpdates domain.PricesResult, quoteDenom string, blockDenomLiquidityUpdatesMap domain.DenomLiquidityMap) domain.PoolDenomMetaDataMap {
	blockTokenMetadataUpdates := make(domain.PoolDenomMetaDataMap, len(blockDenomLiquidityUpdatesMap))

	// Iterate over the denoms updated within the block
	for updatedBlockDenom := range blockDenomLiquidityUpdatesMap {
		// Skip if the denom has a later update than the current height.
		if p.hasLaterUpdateThanCurrent(updatedBlockDenom, uint64(updateHeight)) {
			continue
		}

		// Get the liquidity metadata for the updated denom within the block.
		blockPoolDenomLiquidityData, ok := blockDenomLiquidityUpdatesMap[updatedBlockDenom]
		if !ok {
			// If no denom liquidity metadata available, set the total liquidity & its capitalization to zero.
			blockTokenMetadataUpdates.Set(updatedBlockDenom, osmomath.ZeroInt(), osmomath.ZeroInt())
		}

		quotePrices, ok := blockPriceUpdates[updatedBlockDenom]
		if !ok {
			// If no price is available, keep the total liquidity but set the capitalization to zero.
			blockTokenMetadataUpdates.Set(updatedBlockDenom, blockPoolDenomLiquidityData.TotalLiquidity, osmomath.ZeroInt())
		} else {
			currentBaseScalingFactor, err := p.tokensUseCase.GetChainScalingFactorByDenomMut(updatedBlockDenom)
			if err != nil {
				// If there is an error, silently ignore and skip it.
				continue
			}

			currentPrice, ok := quotePrices[quoteDenom]
			if !ok {
				// If no price is available, keep the total liquidity but set the capitalization to zero.
				blockTokenMetadataUpdates.Set(updatedBlockDenom, blockPoolDenomLiquidityData.TotalLiquidity, osmomath.ZeroInt())
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

		p.latestHeightForDenom.Store(updatedBlockDenom, updateHeight)
	}

	// Return the updated token metadata for testability
	return blockTokenMetadataUpdates
}
