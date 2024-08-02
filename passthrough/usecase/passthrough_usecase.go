package usecase

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsutil/datafetchers"
)

type passthroughUseCase struct {
	poolsUseCase mvc.PoolsUsecase

	tokensUseCase         mvc.TokensUsecase
	defaultQuoteDenom     string
	liquidityPricer       domain.LiquidityPricer
	passthroughGRPCClient passthroughdomain.PassthroughGRPCClient

	aprPrefetcher datafetchers.Fetcher[[]passthroughdomain.PoolAPR]

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

// fetchBalancesPortfolioAssetsJob represents a job to fetch balances for a given category
// in a portfolio assets query.
type fetchBalancesPortfolioAssetsJob struct {
	// name of the category
	name string
	// whether to breakdown the capitalization of the category
	shouldBreakdownCapitalization bool
	// fetchFn is the function to fetch the balances for the category
	fetchFn passthroughdomain.PassthroughFetchFn
}

// coinsResult represents the result of fetching coins
type coinsResult struct {
	// coins fetched
	coins sdk.Coins
	// error encountered during fetching
	err error
}

// totalAssetsCompositionPortfolioAssetsJob represents a job to compose the total portfolio assets
// from the fetched balances.
// Total assets = user balances + staked + unstaking + (pooled - in-locks) + unclaimed-rewards
type totalAssetsCompositionPortfolioAssetsJob struct {
	// name of the category
	name string
	// coins fetched
	coins sdk.Coins
	// any error encountered during the pipiline for any of the categories.
	err error
}

// finalResultPortfolioAssetsJob represents a job to finalize the portfolio assets categories.
type finalResultPortfolioAssetsJob struct {
	// name of the category
	name string
	// result of the category
	result passthroughdomain.PortfolioAssetsCategoryResult
	// any error encountered during the pipiline for constructing the category.
	err error
}

var _ mvc.PassthroughUsecase = &passthroughUseCase{}

const (
	gammSharePrefix         = "gamm"
	concentratedSharePrefix = "cl"
	denomShareSeparator     = "/"
	denomShareSeparatorByte = '/'

	numFinalResultJobs = 7

	totalAssetCompositionNumJobs = 6

	// Number of pooled balance jobs to fetch concurrently.
	// 1. Gamm shares from user balances
	// 2. Concentrated positions
	pooledBalancedNumJobs = 2

	// locked + unlocking
	numInLocksQueries = 2
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

// GetPortfolioBalances implements mvc.PassthroughUsecase.
func (p *passthroughUseCase) GetPortfolioAssets(ctx context.Context, address string) (passthroughdomain.PortfolioAssetsResult, error) {
	// Channel to fetch bank balances concurrently.
	bankBalancesChan := make(chan coinsResult)
	defer close(bankBalancesChan)

	// Channel to fetch pooled balances concurrently.
	// Pool balances arrive from gamm shares and concentrated positions.
	pooledBalancesChan := make(chan coinsResult, pooledBalancedNumJobs)
	defer close(pooledBalancesChan)

	// Channel to fetch unclaimed rewards concurrently.
	unclaimedRewardsChan := make(chan coinsResult)
	defer close(unclaimedRewardsChan)

	go func() {
		// Fetch bank balances and gamm shares concurrently
		bankBalances, gammShareCoins, err := p.getBankBalances(ctx, address)

		// Send the results to the user balances channel
		bankBalancesChan <- coinsResult{
			coins: bankBalances,
			err:   err,
		}

		// Send gamm shares to the pooled balances channel
		pooledBalancesChan <- coinsResult{
			coins: gammShareCoins,
			err:   err,
		}
	}()

	go func() {
		// Fetch concentrated positions and unclaimed rewards concurrently
		positionBalances, unclaimedRewads, err := p.passthroughGRPCClient.UserPositionsBalances(ctx, address)

		// Send the position balances to the pooled balances channel
		pooledBalancesChan <- coinsResult{
			coins: positionBalances,
			err:   err,
		}

		// Send unclaimed rewards to the unclaimed rewards channel
		unclaimedRewardsChan <- coinsResult{
			coins: unclaimedRewads,
			err:   err,
		}
	}()

	// Aggregate poold coins callback
	getPooledCoins := func(ctx context.Context, address string) (sdk.Coins, error) {
		pooledCoins := sdk.Coins{}

		var finalErr error
		for i := 0; i < pooledBalancedNumJobs; i++ {
			pooledCoinsResult := <-pooledBalancesChan
			if pooledCoinsResult.err != nil {
				// Rather than returning the error, log it and continue
				finalErr = pooledCoinsResult.err

				// Ensure that coins are valid to be added and avoid panic.
				if len(pooledCoinsResult.coins) > 0 && !pooledCoinsResult.coins.IsAnyNil() {
					pooledCoins = pooledCoins.Add(pooledCoinsResult.coins...)
				}

				continue
			}

			pooledCoins = pooledCoins.Add(pooledCoinsResult.coins...)
		}

		// Return error and best-effort result
		return pooledCoins, finalErr
	}

	// Callback to fetch bank balances concurrently.
	getBankBalances := func(ctx context.Context, address string) (sdk.Coins, error) {
		bankBalancesResult := <-bankBalancesChan
		return bankBalancesResult.coins, bankBalancesResult.err
	}

	// Callback to fetch unclaimed rewards concurrently.
	getUnclaimedRewards := func(ctx context.Context, address string) (sdk.Coins, error) {
		unclaimedRewardsResult := <-unclaimedRewardsChan
		return unclaimedRewardsResult.coins, unclaimedRewardsResult.err
	}

	// Fetch jobs to fetch the portfolio assets concurrently in separate gorooutines.
	fetchJobs := []fetchBalancesPortfolioAssetsJob{
		{
			name: userBalancesAssetsCategoryName,
			// User balances should be broken down by asset capitalization for each
			// individual coin.
			shouldBreakdownCapitalization: true,
			fetchFn:                       getBankBalances,
		},
		{
			name:    unstakingAssetsCategoryName,
			fetchFn: p.passthroughGRPCClient.DelegatorUnbondingDelegations,
		},
		{
			name:    stakedAssetsCategoryName,
			fetchFn: p.passthroughGRPCClient.DelegatorDelegations,
		},
		{
			name:    inLocksAssetsCategoryName,
			fetchFn: p.getCoinsFromLocks,
		},
		{
			name:    unclaimedRewardsAssetsCategoryName,
			fetchFn: getUnclaimedRewards,
		},
		{
			name:    pooledAssetsCategoryName,
			fetchFn: getPooledCoins,
		},
	}

	totalAssetsCompositionJobs := make(chan totalAssetsCompositionPortfolioAssetsJob, totalAssetCompositionNumJobs)

	finalResultsJobs := make(chan finalResultPortfolioAssetsJob, numFinalResultJobs)
	defer close(finalResultsJobs)

	finalResult := passthroughdomain.PortfolioAssetsResult{
		Categories: make(map[string]passthroughdomain.PortfolioAssetsCategoryResult, numFinalResultJobs),
	}

	for _, fetchJob := range fetchJobs {
		go func(job fetchBalancesPortfolioAssetsJob) {
			// Fetch the balances for the category
			result, finalErr := job.fetchFn(ctx, address)

			if finalErr != nil {
				p.logger.Error("error fetching balances for category", zap.Error(finalErr), zap.String("category", job.name), zap.String("address", address))
			}

			// Send the result to the total assets composition channel
			totalAssetsCompositionJobs <- totalAssetsCompositionPortfolioAssetsJob{
				name:  job.name,
				coins: result,
				err:   finalErr,
			}

			// Skip the category if it is excluded from the final result.
			byAssetCapBreakdown, totalCap, err := p.computeCapitalizationForCoins(ctx, result)
			// Rather than returning the error, persist it and propagate in the pipeline
			// to compute final result.
			if err != nil {
				finalErr = fmt.Errorf("%v, %v", finalErr, err)

				p.logger.Error("error computing capitalization for category", zap.Error(err), zap.String("category", job.name), zap.String("address", address))
			}

			finalJob := finalResultPortfolioAssetsJob{
				name: job.name,
				result: passthroughdomain.PortfolioAssetsCategoryResult{
					Capitalization: totalCap,
					IsBestEffort:   finalErr != nil,
				},
				err: finalErr,
			}

			// Breakdown the capitalization of the category by asset.
			if job.shouldBreakdownCapitalization {
				finalJob.result.AccountCoinsResult = byAssetCapBreakdown
			}

			// Send the final result to the final results channel
			finalResultsJobs <- finalJob
		}(fetchJob)
	}

	go func() {
		totalAssetsCompositionCoins := sdk.Coins{}
		var finalErr error
		for i := 0; i < totalAssetCompositionNumJobs; i++ {
			job := <-totalAssetsCompositionJobs
			if job.err != nil {
				// Attempt to add the coins to the total assets composition
				// even if an error occurred.
				if len(job.coins) > 0 && !job.coins.IsAnyNil() {
					totalAssetsCompositionCoins = totalAssetsCompositionCoins.Add(job.coins...)
				}

				// Rather than returning the error, persist it
				if finalErr == nil {
					finalErr = job.err
				} else {
					finalErr = fmt.Errorf("%v, %v", finalErr, job.err)
				}
				continue
			}

			totalAssetsCompositionCoins = totalAssetsCompositionCoins.Add(job.coins...)
		}

		totalAssetsResult, totalAssetsCap, err := p.computeCapitalizationForCoins(ctx, totalAssetsCompositionCoins)
		if err != nil {
			// Rather than returning the error, persist it
			finalErr = fmt.Errorf("%v, %v", finalErr, err)

			p.logger.Error("error computing total assets capitalization for total assets composition", zap.Error(err), zap.String("address", address))
		}

		finalResultsJobs <- finalResultPortfolioAssetsJob{
			name: totalAssetsCategoryName,
			result: passthroughdomain.PortfolioAssetsCategoryResult{
				Capitalization:     totalAssetsCap,
				AccountCoinsResult: totalAssetsResult,
				IsBestEffort:       finalErr != nil,
			},
			err: finalErr,
		}
	}()

	// Aggregate all results
	// 1. User balances (available) - broken down by asset capitalization
	// 2. Total assets - broken down by asset capitalization
	// 3. Unstaking
	// 4. Staked
	// 5. Unclaimed rewards
	// 6. Pooled
	// 7. In-locks
	for i := 0; i < numFinalResultJobs; i++ {
		job := <-finalResultsJobs
		isBestEffort := job.err != nil
		finalResult.Categories[job.name] = passthroughdomain.PortfolioAssetsCategoryResult{
			IsBestEffort:       isBestEffort,
			AccountCoinsResult: job.result.AccountCoinsResult,
			Capitalization:     job.result.Capitalization,
		}
	}

	return finalResult, nil
}

func (p *passthroughUseCase) RegisterAPRFetcher(aprFetcher datafetchers.Fetcher[[]passthroughdomain.PoolAPR]) {
	p.aprPrefetcher = aprFetcher
}

// computeCapitalizationForCoins instruments the coins with their liquiditiy capitalization values.
// Returns a slice of entries containing each coin and their capialization values. Additionally, returns the capitalization total.
// If coin is not valid, it is skipped from pricing and its capitalization is set to zero.
// Returns error if fails to get prices for the coins. However, a best-effort account coins result is returned even if prices fail to be computed.
func (p *passthroughUseCase) computeCapitalizationForCoins(ctx context.Context, coins sdk.Coins) ([]passthroughdomain.AccountCoinsResult, osmomath.Dec, error) {
	coinDenomsToPrice := make([]string, 0, len(coins))
	for _, coin := range coins {
		if p.tokensUseCase.IsValidChainDenom(coin.Denom) {
			coinDenomsToPrice = append(coinDenomsToPrice, coin.Denom)
		} else {
			p.logger.Debug("denom is not valid & skipped from pricing in portfolio", zap.String("denom", coin.Denom))
		}
	}

	// Compute prices for the final coins
	priceResult, err := p.tokensUseCase.GetPrices(ctx, coinDenomsToPrice, []string{p.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		// Instead of returning an error, attempt to return a best-effort result
		// where all prices are zero.
		priceResult = domain.PricesResult{}
	}

	// Instrument coins with prices
	coinsWithPrices := make([]passthroughdomain.AccountCoinsResult, 0, len(coins))
	capitalizationTotal := osmomath.ZeroDec()

	for _, coin := range coins {
		price := priceResult.GetPriceForDenom(coin.Denom, p.defaultQuoteDenom)

		coinCapitalization := p.liquidityPricer.PriceCoin(coin, price)

		capitalizationTotal = capitalizationTotal.AddMut(coinCapitalization)

		coinsWithPrices = append(coinsWithPrices, passthroughdomain.AccountCoinsResult{
			Coin:                coin,
			CapitalizationValue: coinCapitalization,
		})
	}

	// Note that it is possible to have a valid coinsWithPrices result.
	// Zero capitalizationTotal and non-nil error.
	return coinsWithPrices, capitalizationTotal, err
}

// getLockedCoins returns the user's locked coins
// If encountering GAMM shares, it will convert them to underlying coins
// If encountering concentrated shares, it will skip them
// For every coin, adds the underlying coins to the total coins.
// Returns error if fails to get locked coins.
func (p *passthroughUseCase) getLockedCoins(ctx context.Context, address string, fetchLocksFn passthroughdomain.PassthroughFetchFn) (sdk.Coins, error) {
	// User locked/unlocking assets including GAMM shares
	lockedCoins, err := fetchLocksFn(ctx, address)
	if err != nil {
		return nil, err
	}

	coins := sdk.Coins{}

	for _, lockedCoin := range lockedCoins {
		// calc underlying coins from GAMM shares, only expect gamm shares
		accumulated, err := p.tryAccumulateGammShares(&coins, lockedCoin)
		if accumulated || err != nil {
			continue
		}
		// Concentrated value is retrieved from positions.
		// As a result, we skip them here.
		if !strings.HasPrefix(lockedCoin.Denom, concentratedSharePrefix) {
			coins = coins.Add(lockedCoin)
		}
	}

	return coins, nil
}

// getCoinsFromLocks returns the user's coins from locks
// Returns both locked and unlocking coins.
// If encountering GAMM shares, it will convert them to underlying coins
// If encountering concentrated shares, it will skip them
// For every coin, adds the underlying coins to the total coins.
// Returns error if fails to get locked coins but the best-effort result is returned still
func (p *passthroughUseCase) getCoinsFromLocks(ctx context.Context, address string) (sdk.Coins, error) {
	result := make(chan coinsResult, numInLocksQueries)
	defer close(result)

	for _, fetchLocksFn := range []passthroughdomain.PassthroughFetchFn{
		p.passthroughGRPCClient.AccountLockedCoins,
		p.passthroughGRPCClient.AccountUnlockingCoins,
	} {
		go func(fetchLocksFn passthroughdomain.PassthroughFetchFn) {
			lockedCoins, err := p.getLockedCoins(ctx, address, fetchLocksFn)
			result <- coinsResult{
				coins: lockedCoins,
				err:   err,
			}
		}(fetchLocksFn)
	}

	var (
		coinsResult = sdk.Coins{}
		finalErr    error
	)

	for i := 0; i < numInLocksQueries; i++ {
		res := <-result
		if res.err != nil {
			// Skip silently and continue
			finalErr = res.err
			continue
		}

		coinsResult = coinsResult.Add(res.coins...)
	}

	// Return best-effort result and error.
	return coinsResult, finalErr
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
