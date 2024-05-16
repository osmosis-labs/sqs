package worker

import (
	"context"
	"fmt"
	"strings"
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

	latestHeightForDenom sync.Map
}

type DenomPriceInfo struct {
	Price         osmomath.BigDec
	ScalingFactor osmomath.Dec
}

func NewPoolLiquidityWorker(tokensUseCase mvc.TokensUsecase, poolsUseCase mvc.PoolsUsecase) domain.PricingUpdateListener {
	return &poolLiquidityComputeWorker{

		poolsUseCase:  poolsUseCase,
		tokensUseCase: tokensUseCase,

		latestHeightForDenom: sync.Map{},
	}
}

// OnPricingUpdate implements worker.PricingUpdateListener.
// CONTRACT: QueuePoolLiquidityCompute with the same height must be called prior to this.
func (p *poolLiquidityComputeWorker) OnPricingUpdate(ctx context.Context, height int64, blockPoolMetadata domain.BlockPoolMetadata, baseDenomPriceUpdates map[string]map[string]osmomath.BigDec, quoteDenom string) error {
	// Compute the scaling factors for the base denoms.
	baseDenomPriceData := make(map[string]DenomPriceInfo, len(baseDenomPriceUpdates))
	for baseDenom, quotesPrices := range baseDenomPriceUpdates {
		price, ok := quotesPrices[quoteDenom]
		if !ok {
			return fmt.Errorf("no price update for %s when computing pool TVL", quoteDenom)
		}

		baseScalingFactor, err := p.tokensUseCase.GetChainScalingFactorByDenomMut(baseDenom)
		if err != nil {
			return err
		}

		baseDenomPriceData[baseDenom] = DenomPriceInfo{
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
			usdcLiquidityValue, err := computeCoinTVL(sdk.NewCoin(denom, poolDenomMetaData.TotalLiquidity), price)
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

// nolint: unused
func computeBalanceTVL(balance sdk.Coins, baseDenomPriceData map[string]DenomPriceInfo) (osmomath.Int, string) {
	usdcTVL := osmomath.ZeroDec()

	tvlError := ""

	for _, balance := range balance {
		// // TODO: for some reason we halt on these (potentially no routes)
		// if balance.Denom == "ibc/E610B83FD5544E00A8A1967A2EB3BEF25F1A8CFE8650FE247A8BD4ECA9DC9222" || balance.Denom == "ibc/B2BD584CD2A0A9CE53D4449667E26160C7D44A9C41AF50F602C201E5B3CCA46C" {
		// 	continue
		// }

		// Skip gamm shares
		if strings.Contains(balance.Denom, "gamm/pool") {
			continue
		}

		baseDenomPrice, ok := baseDenomPriceData[balance.Denom]
		if !ok {
			tvlError += fmt.Sprintf(" %s", fmt.Errorf("no price update for %s when computing pool TVL", balance.Denom))
			continue
		}

		currentTokenTVL, err := computeCoinTVL(balance, baseDenomPrice)
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
func computeCoinTVL(coin sdk.Coin, baseDenomPriceData DenomPriceInfo) (osmomath.Dec, error) {
	if baseDenomPriceData.Price.IsZero() {
		return osmomath.Dec{}, fmt.Errorf("price for %s is zero", coin.Denom)
	}
	if baseDenomPriceData.ScalingFactor.IsZero() {
		return osmomath.Dec{}, fmt.Errorf("scaling factor for %s is zero", coin.Denom)
	}

	currentTokenTVL := osmomath.NewBigDecFromBigInt(coin.Amount.BigIntMut()).MulMut(baseDenomPriceData.Price)
	isOriginalAmountZero := coin.Amount.IsZero()

	// Truncation in intermediary operation - return error.
	if currentTokenTVL.IsZero() && !isOriginalAmountZero {
		return osmomath.Dec{}, fmt.Errorf("truncation occurred when multiplying coin (%s) by price %s", coin, baseDenomPriceData.Price)
	}

	// Truncation in intermediary operation - return error.
	currentTokenTVL = currentTokenTVL.QuoMut(osmomath.BigDecFromDec(baseDenomPriceData.ScalingFactor))
	if currentTokenTVL.IsZero() && !isOriginalAmountZero {
		return osmomath.Dec{}, fmt.Errorf("truncation occurred when multiplying (%s) of denom (%s) by the scaling factor (%s)", currentTokenTVL, coin.Denom, baseDenomPriceData.ScalingFactor)
	}

	if currentTokenTVL.IsZero() {
		fmt.Println("currentTokenTVL is zero ", coin, " ", baseDenomPriceData.Price, " ", baseDenomPriceData.ScalingFactor)
	}

	return currentTokenTVL.Dec(), nil
}
