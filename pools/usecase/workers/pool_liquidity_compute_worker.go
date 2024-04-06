package workers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
)

type PoolLiquidityComputeWorker interface {
	worker.PricingUpdateListener

	QueuePoolLiquidityCompute(ctx context.Context, height uint64, poolIDs []uint64)
}

type PoolLiquidityComputeListener interface {
	OnPoolLiquidityCompute(height int64, updatedPoolIDs []uint64) error
}

var (
	_ PoolLiquidityComputeWorker   = &poolLiquidityComputeWorker{}
	_ worker.PricingUpdateListener = &poolLiquidityComputeWorker{}
)

type poolLiquidityComputeWorker struct {
	pendingPoolLiquidityUpdateMu sync.Mutex
	pendingPoolLiquidityUpdates  map[int64][]uint64

	poolsUseCase  mvc.PoolsUsecase
	tokensUseCase mvc.TokensUsecase
}

type DenomPriceInfo struct {
	Price         osmomath.BigDec
	ScalingFactor osmomath.Dec
}

func New(tokensUseCase mvc.TokensUsecase, poolsUseCase mvc.PoolsUsecase) PoolLiquidityComputeWorker {
	return &poolLiquidityComputeWorker{
		pendingPoolLiquidityUpdateMu: sync.Mutex{},
		pendingPoolLiquidityUpdates:  map[int64][]uint64{},

		poolsUseCase:  poolsUseCase,
		tokensUseCase: tokensUseCase,
	}
}

// OnPricingUpdate implements worker.PricingUpdateListener.
// CONTRACT: QueuePoolLiquidityCompute with the same height must be called prior to this.
func (p *poolLiquidityComputeWorker) OnPricingUpdate(ctx context.Context, height int64, baseDenomPriceUpdates map[string]osmomath.BigDec, quoteDenom string) error {
	p.pendingPoolLiquidityUpdateMu.Lock()
	poolIDs, ok := p.pendingPoolLiquidityUpdates[height]
	delete(p.pendingPoolLiquidityUpdates, height)
	p.pendingPoolLiquidityUpdateMu.Unlock()

	if !ok {
		return fmt.Errorf("no pending pool liquidity updates for height %d", height)
	}

	// Compute the scaling factors for the base denoms.
	baseDenomPriceData := make(map[string]DenomPriceInfo, len(baseDenomPriceUpdates))
	for baseDenom, price := range baseDenomPriceUpdates {
		baseScalingFactor, err := p.tokensUseCase.GetChainScalingFactorByDenomMut(ctx, baseDenom)
		if err != nil {
			return err
		}

		baseDenomPriceData[baseDenom] = DenomPriceInfo{
			Price:         price,
			ScalingFactor: baseScalingFactor,
		}
	}

	// For every pool compute total liqudity given the prices and scaling factors.
	for _, pool := range poolIDs {
		pool, err := p.poolsUseCase.GetPool(pool)
		if err != nil {
			return err
		}

		balanceTotalValueUSDC, liquidityErr := computeBalanceTVL(pool.GetSQSPoolModel().Balances, baseDenomPriceData)

		pool.SetTotalValueLockedUSDC(balanceTotalValueUSDC)
		pool.SetTotalValueLockedError(liquidityErr)

		if err := p.poolsUseCase.StorePools([]sqsdomain.PoolI{pool}); err != nil {
			return err
		}
	}

	// TODO: async compute pool liquidity

	return nil
}

// QueuePoolLiquidityCompute implements PoolLiquidityComputeWorker.
func (p *poolLiquidityComputeWorker) QueuePoolLiquidityCompute(ctx context.Context, height uint64, poolIDs []uint64) {
	p.pendingPoolLiquidityUpdateMu.Lock()
	p.pendingPoolLiquidityUpdates[int64(height)] = poolIDs
	p.pendingPoolLiquidityUpdateMu.Unlock()
}

func computeBalanceTVL(poolBalances sdk.Coins, baseDenomPriceData map[string]DenomPriceInfo) (osmomath.Int, string) {
	usdcTVL := osmomath.ZeroDec()

	tvlError := ""

	for _, balance := range poolBalances {
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

	return currentTokenTVL.Dec(), nil
}
