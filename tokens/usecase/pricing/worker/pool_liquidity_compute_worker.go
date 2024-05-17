package worker

import (
	"context"
	"fmt"
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type PoolLiquidityComputeListener interface {
	OnPoolLiquidityCompute(height int64, updatedPoolIDs []uint64) error
}

var (
	_ domain.PricingUpdateListener = &poolLiquidityComputeWorker{}
)

type poolLiquidityComputeWorker struct {
	poolsUseCase  mvc.PoolsUsecase
	tokensUseCase mvc.TokensUsecase

	liquidityPricer domain.LiquidityPricer

	latestHeightForDenom sync.Map
}

func NewPoolLiquidityWorker(tokensUseCase mvc.TokensUsecase, poolsUseCase mvc.PoolsUsecase, liquidityPricer domain.LiquidityPricer) domain.PricingUpdateListener {
	return &poolLiquidityComputeWorker{

		poolsUseCase:  poolsUseCase,
		tokensUseCase: tokensUseCase,

		liquidityPricer: liquidityPricer,

		latestHeightForDenom: sync.Map{},
	}
}

// OnPricingUpdate implements worker.PricingUpdateListener.
// CONTRACT: QueuePoolLiquidityCompute with the same height must be called prior to this.
func (p *poolLiquidityComputeWorker) OnPricingUpdate(ctx context.Context, height int64, blockPoolMetadata domain.BlockPoolMetadata, baseDenomPriceUpdates map[string]map[string]osmomath.BigDec, quoteDenom string) error {
	// Compute the scaling factors for the base denoms.
	baseDenomPriceData := make(map[string]domain.DenomPriceInfo, len(baseDenomPriceUpdates))
	for baseDenom, quotesPrices := range baseDenomPriceUpdates {
		price, ok := quotesPrices[quoteDenom]
		if !ok {
			return fmt.Errorf("no price update for %s when computing pool TVL", quoteDenom)
		}

		baseScalingFactor, err := p.tokensUseCase.GetChainScalingFactorByDenomMut(baseDenom)
		if err != nil {
			return err
		}

		baseDenomPriceData[baseDenom] = domain.DenomPriceInfo{
			Price:         price,
			ScalingFactor: baseScalingFactor,
		}
	}

	tokensMetadata := make(map[string]domain.PoolDenomMetaData, len(blockPoolMetadata.UpdatedDenoms))

	// Iterate over the denoms updated within the block
	for denom := range blockPoolMetadata.UpdatedDenoms {
		latestHeightForDenomObj, ok := p.latestHeightForDenom.Load(denom)
		if ok {
			// Skip if the height is not the latest.
			latestHeightForDenom, ok := latestHeightForDenomObj.(int64)
			if !ok || height < latestHeightForDenom {
				continue
			}
		}

		poolDenomMetaData, ok := blockPoolMetadata.DenomLiquidityMap[denom]
		if !ok {
			// If no denom liquidity metadata available, set the total liquidity to zero.
			tokensMetadata[denom] = domain.PoolDenomMetaData{
				TotalLiquidity:     osmomath.ZeroInt(),
				TotalLiquidityUSDC: osmomath.ZeroInt(),
			}
		}

		price, ok := baseDenomPriceData[denom]
		if !ok {
			// If no price is available, set the total liquidity to zero.
			tokensMetadata[denom] = domain.PoolDenomMetaData{
				TotalLiquidity:     poolDenomMetaData.TotalLiquidity,
				TotalLiquidityUSDC: osmomath.ZeroInt(),
			}
		} else {
			usdcLiquidityValue, err := p.liquidityPricer.ComputeCoinCap(sdk.NewCoin(denom, poolDenomMetaData.TotalLiquidity), price)
			if err != nil {
				// If there is an error, set the total liquidity to zero.
				tokensMetadata[denom] = domain.PoolDenomMetaData{
					TotalLiquidity:     poolDenomMetaData.TotalLiquidity,
					TotalLiquidityUSDC: osmomath.ZeroInt(),
				}
			} else {
				// Set the total liquidity in USDC.
				tokensMetadata[denom] = domain.PoolDenomMetaData{
					TotalLiquidity:     poolDenomMetaData.TotalLiquidity,
					TotalLiquidityUSDC: usdcLiquidityValue.TruncateInt(),
				}
			}
		}

		p.latestHeightForDenom.Store(denom, height)
	}

	p.tokensUseCase.UpdatePoolDenomMetadata(tokensMetadata)

	return nil
}
