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
	// This exists because pricing computations are asyncronous. As a result, a pricing update for a later
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
	tokensMetadata := make(map[string]domain.PoolDenomMetaData, len(blockPoolMetadata.UpdatedDenoms))

	// Iterate over the denoms updated within the block
	for updatedBlockDenom := range blockPoolMetadata.UpdatedDenoms {
		// Skip if the denom has a later update than the current height.
		if p.hasLaterUpdateThanCurrent(updatedBlockDenom, uint64(height)) {
			continue
		}

		blockPoolDenomMetaData, ok := blockPoolMetadata.DenomLiquidityMap[updatedBlockDenom]
		if !ok {
			// If no denom liquidity metadata available, set the total liquidity to zero.
			tokensMetadata[updatedBlockDenom] = domain.PoolDenomMetaData{
				TotalLiquidity:     osmomath.ZeroInt(),
				TotalLiquidityUSDC: osmomath.ZeroInt(),
			}
		}

		quotePrices, ok := baseDenomPriceUpdates[updatedBlockDenom]
		if !ok {
			// If no price is available, set the total liquidity to zero.
			tokensMetadata[updatedBlockDenom] = domain.PoolDenomMetaData{
				TotalLiquidity:     blockPoolDenomMetaData.TotalLiquidity,
				TotalLiquidityUSDC: osmomath.ZeroInt(),
			}
		} else {
			currentBaseScalingFactor, err := p.tokensUseCase.GetChainScalingFactorByDenomMut(updatedBlockDenom)
			if err != nil {
				return err
			}

			currentPrice, ok := quotePrices[quoteDenom]
			if !ok {
				// If no price is available, set the total USDC liquidity to zero.
				tokensMetadata[updatedBlockDenom] = domain.PoolDenomMetaData{
					TotalLiquidity:     blockPoolDenomMetaData.TotalLiquidity,
					TotalLiquidityUSDC: osmomath.ZeroInt(),
				}
			}

			currentPriceInfo := domain.DenomPriceInfo{
				Price:         currentPrice,
				ScalingFactor: currentBaseScalingFactor,
			}

			usdcLiquidityValue, err := p.liquidityPricer.ComputeCoinCap(sdk.NewCoin(updatedBlockDenom, blockPoolDenomMetaData.TotalLiquidity), currentPriceInfo)
			if err != nil {
				// If there is an error, set the total liquidity to zero.
				tokensMetadata[updatedBlockDenom] = domain.PoolDenomMetaData{
					TotalLiquidity:     blockPoolDenomMetaData.TotalLiquidity,
					TotalLiquidityUSDC: osmomath.ZeroInt(),
				}
			} else {
				// Set the total liquidity in USDC.
				tokensMetadata[updatedBlockDenom] = domain.PoolDenomMetaData{
					TotalLiquidity:     blockPoolDenomMetaData.TotalLiquidity,
					TotalLiquidityUSDC: usdcLiquidityValue.TruncateInt(),
				}
			}
		}

		p.latestHeightForDenom.Store(updatedBlockDenom, height)
	}

	p.tokensUseCase.UpdatePoolDenomMetadata(tokensMetadata)

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
