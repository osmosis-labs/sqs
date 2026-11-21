package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/domain/recovery"
	"github.com/osmosis-labs/sqs/log"
)

type passthroughUseCase struct {
	poolsUseCase mvc.PoolsUsecase

	tokensUseCase         mvc.TokensUsecase
	defaultQuoteDenom     string
	liquidityPricer       domain.LiquidityPricer
	passthroughGRPCClient passthroughdomain.PassthroughGRPCClient

	logger log.Logger
}

const (
	userBalancesAssetsCategoryName     string = "user-balances"
	unstakingAssetsCategoryName        string = "unstaking"
	stakedAssetsCategoryName           string = "staked"
	inLocksAssetsCategoryName          string = "in-locks"
	pooledAssetsCategoryName           string = "pooled"
	unclaimedRewardsAssetsCategoryName string = "unclaimed-rewards"
	totalAssetsCategoryName            string = "total-assets"
)

var _ mvc.PassthroughUsecase = &passthroughUseCase{}

const (
	gammSharePrefix         = "gamm"
	concentratedSharePrefix = "cl"
	denomShareSeparatorByte = '/'

	// portfolioAssetsRequestTimeout bounds the fan-out chain gRPC calls for one
	// /passthrough/portfolio-assets request. It stays below the HTTP server write timeout
	// so stalled gRPC calls cannot leak goroutines after the socket deadline expires.
	portfolioAssetsRequestTimeout = 30 * time.Second
)

// NewPassThroughUsecase Creates a passthrough use case
func NewPassThroughUsecase(passthroughGRPCClient passthroughdomain.PassthroughGRPCClient, puc mvc.PoolsUsecase, tokensUseCase mvc.TokensUsecase, liquidityPricer domain.LiquidityPricer, defaultQuoteDenom string, logger log.Logger) *passthroughUseCase {
	return &passthroughUseCase{
		poolsUseCase: puc,

		passthroughGRPCClient: passthroughGRPCClient,

		tokensUseCase:     tokensUseCase,
		defaultQuoteDenom: defaultQuoteDenom,
		liquidityPricer:   liquidityPricer,

		logger: logger,
	}
}

// GetPortfolioAssets implements mvc.PassthroughUsecase.
func (p *passthroughUseCase) GetPortfolioAssets(ctx context.Context, address string) (passthroughdomain.PortfolioAssetsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, portfolioAssetsRequestTimeout)
	defer cancel()

	var (
		bankCoins, gammShareCoins          sdk.Coins
		bankErr                            error
		positionCoins, positionRewardCoins sdk.Coins
		positionErr                        error
		stakingRewardCoins                 sdk.Coins
		stakingRewardErr                   error
		unstakingCoins                     sdk.Coins
		unstakingErr                       error
		stakedCoins                        sdk.Coins
		stakedErr                          error
		inLocksCoins                       sdk.Coins
		inLocksErr                         error
	)

	// Phase 1: fetch all independent chain queries concurrently.
	var wg sync.WaitGroup
	wg.Add(6)
	go func() {
		defer wg.Done()
		bankCoins, gammShareCoins, bankErr = recoverCoinsPair(func() (sdk.Coins, sdk.Coins, error) {
			return p.getBankBalances(ctx, address)
		})
	}()
	go func() {
		defer wg.Done()
		positionCoins, positionRewardCoins, positionErr = recoverCoinsPair(func() (sdk.Coins, sdk.Coins, error) {
			return p.passthroughGRPCClient.UserPositionsBalances(ctx, address)
		})
	}()
	go func() {
		defer wg.Done()
		stakingRewardCoins, stakingRewardErr = recoverCoins(func() (sdk.Coins, error) {
			return p.passthroughGRPCClient.DelegationRewards(ctx, address)
		})
	}()
	go func() {
		defer wg.Done()
		unstakingCoins, unstakingErr = recoverCoins(func() (sdk.Coins, error) {
			return p.passthroughGRPCClient.DelegatorUnbondingDelegations(ctx, address)
		})
	}()
	go func() {
		defer wg.Done()
		stakedCoins, stakedErr = recoverCoins(func() (sdk.Coins, error) {
			return p.passthroughGRPCClient.DelegatorDelegations(ctx, address)
		})
	}()
	go func() {
		defer wg.Done()
		inLocksCoins, inLocksErr = recoverCoins(func() (sdk.Coins, error) {
			return p.getCoinsFromLocks(ctx, address)
		})
	}()
	wg.Wait()

	// Phase 2: combine categories, compute total coins, and price everything in one GetPrices call.
	pooledCoins, pooledErr := mergeCoins(gammShareCoins, bankErr, positionCoins, positionErr)
	unclaimedCoins, unclaimedErr := mergeCoins(positionRewardCoins, positionErr, stakingRewardCoins, stakingRewardErr)

	type categoryCoins struct {
		name      string
		coins     sdk.Coins
		err       error
		breakdown bool
	}

	categories := []categoryCoins{
		{name: userBalancesAssetsCategoryName, coins: bankCoins, err: bankErr, breakdown: true},
		{name: unstakingAssetsCategoryName, coins: unstakingCoins, err: unstakingErr},
		{name: stakedAssetsCategoryName, coins: stakedCoins, err: stakedErr},
		{name: inLocksAssetsCategoryName, coins: inLocksCoins, err: inLocksErr},
		{name: unclaimedRewardsAssetsCategoryName, coins: unclaimedCoins, err: unclaimedErr},
		{name: pooledAssetsCategoryName, coins: pooledCoins, err: pooledErr},
	}

	totalCoins := sdk.Coins{}
	var totalErr error
	for _, cat := range categories {
		if cat.err != nil {
			p.logger.Error("error fetching balances for category", zap.Error(cat.err), zap.String("category", cat.name), zap.String("address", address))
			totalErr = errors.Join(totalErr, cat.err)
		}
		if len(cat.coins) > 0 && !cat.coins.IsAnyNil() {
			var addErr error
			totalCoins, addErr = addCoins(totalCoins, cat.coins)
			totalErr = errors.Join(totalErr, addErr)
		}
	}

	categories = append(categories, categoryCoins{
		name:      totalAssetsCategoryName,
		coins:     totalCoins,
		err:       totalErr,
		breakdown: true,
	})

	// Collect all unique valid denoms once and fetch prices in a single batched call.
	priceResult, priceErr := p.fetchPricesForCoins(ctx, bankCoins, unstakingCoins, stakedCoins, inLocksCoins, unclaimedCoins, pooledCoins)

	finalResult := passthroughdomain.PortfolioAssetsResult{
		Categories: make(map[string]passthroughdomain.PortfolioAssetsCategoryResult, len(categories)),
	}
	for _, cat := range categories {
		breakdown, totalCap, capErr := p.priceCoins(cat.coins, priceResult, priceErr)
		if capErr != nil {
			p.logger.Error("error computing capitalization for category", zap.Error(capErr), zap.String("category", cat.name), zap.String("address", address))
		}

		res := passthroughdomain.PortfolioAssetsCategoryResult{
			Capitalization: totalCap,
			IsBestEffort:   cat.err != nil || capErr != nil,
		}
		if cat.breakdown {
			res.AccountCoinsResult = breakdown
		}
		finalResult.Categories[cat.name] = res
	}

	return finalResult, nil
}

// fetchPricesForCoins deduplicates valid denoms across coinSets and fetches their prices in one call.
func (p *passthroughUseCase) fetchPricesForCoins(ctx context.Context, coinSets ...sdk.Coins) (domain.PricesResult, error) {
	seenDenoms := make(map[string]struct{})
	var denomsToPrice []string
	for _, coins := range coinSets {
		for _, coin := range coins {
			if _, ok := seenDenoms[coin.Denom]; ok {
				continue
			}
			seenDenoms[coin.Denom] = struct{}{}
			if p.tokensUseCase.IsValidChainDenom(coin.Denom) {
				denomsToPrice = append(denomsToPrice, coin.Denom)
			} else {
				p.logger.Debug("denom is not valid & skipped from pricing in portfolio", zap.String("denom", coin.Denom))
			}
		}
	}

	priceResult, err := p.tokensUseCase.GetPrices(ctx, denomsToPrice, []string{p.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return domain.PricesResult{}, err
	}
	return priceResult, nil
}

func (p *passthroughUseCase) priceCoins(coins sdk.Coins, priceResult domain.PricesResult, priceErr error) (_ []passthroughdomain.AccountCoinsResult, _ osmomath.Dec, err error) {
	defer recovery.ToErrorf(&err, "computing capitalization")

	err = priceErr
	coinsWithPrices := make([]passthroughdomain.AccountCoinsResult, 0, len(coins))
	capitalizationTotal := osmomath.ZeroDec()

	var unpricedDenoms []string
	for _, coin := range coins {
		price := priceResult.GetPriceForDenom(coin.Denom, p.defaultQuoteDenom)

		// A valid denom without a price contributes zero, so the total is only best-effort.
		if _, ok := priceResult[coin.Denom][p.defaultQuoteDenom]; !ok && err == nil && p.tokensUseCase.IsValidChainDenom(coin.Denom) {
			unpricedDenoms = append(unpricedDenoms, coin.Denom)
		}

		coinCapitalization := p.liquidityPricer.PriceCoin(coin, price)
		capitalizationTotal = capitalizationTotal.AddMut(coinCapitalization)
		coinsWithPrices = append(coinsWithPrices, passthroughdomain.AccountCoinsResult{
			Coin:                coin,
			CapitalizationValue: coinCapitalization,
		})
	}

	if len(unpricedDenoms) > 0 {
		err = fmt.Errorf("no price for denoms: %s", strings.Join(unpricedDenoms, ", "))
	}

	return coinsWithPrices, capitalizationTotal, err
}

// getCoinsFromLocks returns the user's locked coins (which already include unlocking coins).
// GAMM shares are converted to underlying coins and concentrated shares are skipped.
func (p *passthroughUseCase) getCoinsFromLocks(ctx context.Context, address string) (sdk.Coins, error) {
	// AccountLockedCoins already includes locks that are unlocking, so no separate unlocking query.
	lockedCoins, err := p.passthroughGRPCClient.AccountLockedCoins(ctx, address)
	if err != nil {
		return sdk.Coins{}, err
	}

	coins := sdk.Coins{}
	for _, lockedCoin := range lockedCoins {
		accumulated, err := p.tryAccumulateGammShares(&coins, lockedCoin)
		if accumulated || err != nil {
			continue
		}
		// Concentrated value is retrieved from positions.
		if !strings.HasPrefix(lockedCoin.Denom, concentratedSharePrefix) {
			coins = coins.Add(lockedCoin)
		}
	}

	return coins, nil
}

// getBankBalances returns the user's bank balances
// If encountering GAMM shares, it will convert them to underlying coins
// Returns error if fails to get bank balances.
// For every coin, adds the underlying coins to the total coins.
// If the coin is not a GAMM share, it is added as is.
// If the coin is a GAMM share, it is converted to underlying coins and adds them.
// If any error occurs during the conversion, it is logged and skipped silently.
func (p *passthroughUseCase) getBankBalances(ctx context.Context, address string) (sdk.Coins, sdk.Coins, error) {
	allBalances, err := p.passthroughGRPCClient.AllBalances(ctx, address)
	if err != nil {
		// This error is not expected and is considered fatal
		// To be handled as needed by the caller.
		return nil, nil, err
	}

	gammShareCoins := sdk.Coins{}
	balanceCoins := sdk.Coins{}

	for _, balance := range allBalances {
		// calc underlying coins from GAMM shares, only expect gamm shares
		accumulated, err := p.tryAccumulateGammShares(&gammShareCoins, balance)
		if accumulated || err != nil {
			continue
		}
		// Performance optimization to avoid sorting with Add(...)
		balanceCoins = append(balanceCoins, balance)
	}

	return balanceCoins.Sort(), gammShareCoins, nil
}

// handleGammShares converts GAMM shares to underlying coins
// Returns error if fails to convert GAMM shares to underlying coins.
// Returns the underlying coins if successful.
// CONTRACT: coin is a gamm share
func (p *passthroughUseCase) handleGammShares(coin sdk.Coin) (sdk.Coins, error) {
	// calc underlying coins from gamm shares
	poolIDStart := strings.LastIndexByte(coin.Denom, denomShareSeparatorByte) + 1
	poolIDInt, err := strconv.ParseUint(coin.Denom[poolIDStart:], 10, 64)
	if err != nil {
		return sdk.Coins{}, err
	}

	exitCoins, err := p.poolsUseCase.CalcExitCFMMPool(poolIDInt, coin.Amount)
	if err != nil {
		return sdk.Coins{}, err
	}

	return exitCoins, nil
}

func (p *passthroughUseCase) tryAccumulateGammShares(coinsTarget *sdk.Coins, coin sdk.Coin) (isGammShare bool, err error) {
	if strings.HasPrefix(coin.Denom, gammSharePrefix) {
		exitCoins, err := p.handleGammShares(coin)
		if err != nil {
			p.logger.Error("error converting gamm share from balances to underlying coins", zap.Error(err))
			return true, err
		}

		target := *coinsTarget
		target = target.Add(exitCoins...)
		*coinsTarget = target
		return true, nil
	}
	return false, nil
}

// recoverCoins runs fetch and returns its result, turning a panic into an error.
//
// The portfolio fetches run in goroutines of their own, where a panic is not caught by the
// HTTP recover middleware and would take the whole process down. As an error, it only marks
// the affected category as best effort.
func recoverCoins(fetch func() (sdk.Coins, error)) (coins sdk.Coins, err error) {
	defer recovery.ToErrorf(&err, "fetching portfolio assets")

	return fetch()
}

// recoverCoinsPair is recoverCoins for a fetch that returns two sets of coins.
func recoverCoinsPair(fetch func() (sdk.Coins, sdk.Coins, error)) (first sdk.Coins, second sdk.Coins, err error) {
	defer recovery.ToErrorf(&err, "fetching portfolio assets")

	return fetch()
}

// addCoins returns total plus coins. Coins.Add panics on coins that are not sorted or not
// valid; in that case total is returned unchanged, with an error.
func addCoins(total, coins sdk.Coins) (sum sdk.Coins, err error) {
	// On a panic, sum keeps this value.
	sum = total
	defer recovery.ToErrorf(&err, "adding coins (%s)", coins)

	return total.Add(coins...), nil
}

// mergeCoins combines two best-effort coin fetch results into one.
func mergeCoins(first sdk.Coins, firstErr error, second sdk.Coins, secondErr error) (sdk.Coins, error) {
	merged := sdk.Coins{}
	err := errors.Join(firstErr, secondErr)
	for _, coins := range []sdk.Coins{first, second} {
		if len(coins) == 0 || coins.IsAnyNil() {
			continue
		}
		var addErr error
		merged, addErr = addCoins(merged, coins)
		err = errors.Join(err, addErr)
	}
	return merged, err
}
