package usecase

import (
	"context"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
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

var _ mvc.PassthroughUsecase = &passthroughUseCase{}

const (
	gammSharePrefix         = "gamm"
	concentratedSharePrefix = "cl"
	denomShareSeparator     = "/"
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
	fetchFuncs := []passthroughdomain.PassthroughFetchFn{
		p.passthroughGRPCClient.DelegatorUnbondingDelegations,
		p.passthroughGRPCClient.DelegatorDelegations,
		p.getLockedCoins,
		p.passthroughGRPCClient.UserPositionsBalances,
	}

	balancesFn := []passthroughdomain.PassthroughFetchFn{
		p.getBankBalances,
	}

	totalCapResultChan := make(chan passthroughdomain.PortfolioAssetsResult, 2)
	errs := make(chan error, 2)

	go func() {
		// Process all non-balance assets (cl posiions, staked, locked, etc.)
		portfolioAssetsResult, err := p.fetchAndAggregateBalancesByUserConcurrent(ctx, address, fetchFuncs)
		if err != nil {
			errs <- err
			return
		}
		totalCapResultChan <- portfolioAssetsResult
	}()

	// Process user balances
	balancesResult, err := p.fetchAndAggregateBalancesByUserConcurrent(ctx, address, balancesFn)
	if err != nil {
		errs <- err
	} else {
		totalCapResultChan <- balancesResult
	}

	// Aggregate total capitalization
	totalCap := osmomath.ZeroDec()
	for i := 0; i < 2; i++ {
		select {
		case res := <-totalCapResultChan:
			totalCap = totalCap.Add(res.TotalValueCap)
		case err := <-errs:
			// Rather than returning the error, log it and continue
			p.logger.Error("error fetching and aggregating porfolio", zap.Error(err))
			continue
		}
	}

	close(totalCapResultChan)
	close(errs)

	return passthroughdomain.PortfolioAssetsResult{
		TotalValueCap:      totalCap,
		AccountCoinsResult: balancesResult.AccountCoinsResult,
	}, nil
}

func (p *passthroughUseCase) fetchAndAggregateBalancesByUserConcurrent(ctx context.Context, address string, fetchFunctions []passthroughdomain.PassthroughFetchFn) (passthroughdomain.PortfolioAssetsResult, error) {
	coins := sdk.Coins{}

	numAccountCoinsFetchFunctons := len(fetchFunctions)
	results := make(chan sdk.Coins, numAccountCoinsFetchFunctons)
	errs := make(chan error, numAccountCoinsFetchFunctons)

	for _, fetchFunc := range fetchFunctions {
		go func(fetchFunc func(context.Context, string) (sdk.Coins, error)) {
			result, err := fetchFunc(ctx, address)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}(fetchFunc)
	}

	// Final error that we return
	var finalErr error

	for i := 0; i < len(fetchFunctions); i++ {
		select {
		case res := <-results:

			coins = coins.Add(res...)
		case curErr := <-errs:
			// Rather than returning the error, log it and continue
			p.logger.Error("error fetching and aggregating balances", zap.Error(curErr))

			// Set the last error as the final error
			finalErr = curErr
		}
	}

	close(results)
	close(errs)

	accountCoinsResult, capitalizationTotal, err := p.computeCapitalizationForCoins(ctx, coins)
	if err != nil {
		p.logger.Error("error instrumenting coins with prices when retrieving portfolio", zap.Error(err))
		finalErr = err
	}

	return passthroughdomain.PortfolioAssetsResult{
		AccountCoinsResult: accountCoinsResult,
		TotalValueCap:      capitalizationTotal,
	}, finalErr // Note that we skip all errors for best-effort aggregation but propagate the last one observed, if any.
}

// computeCapitalizationForCoins instruments the coins with their liquiditiy capitalization values.
// Returns a slice of entries containing each coin and their capialization values. Additonally, returns the capitalization total.
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
func (p *passthroughUseCase) getLockedCoins(ctx context.Context, address string) (sdk.Coins, error) {
	// User locked assets including GAMM shares
	lockedCoins, err := p.passthroughGRPCClient.AccountLockedCoins(ctx, address)
	if err != nil {
		return nil, err
	}

	coins := sdk.Coins{}

	for _, lockedCoin := range lockedCoins {
		// calc underlying coins from GAMM shares, only expect gamm shares
		if strings.HasPrefix(lockedCoin.Denom, gammSharePrefix) {
			exitCoins, err := p.handleGammShares(lockedCoin)
			if err != nil {
				p.logger.Error("error converting gamm share from locks to underlying coins", zap.Error(err))
				continue
			}

			coins = coins.Add(exitCoins...)

			// Concentrated value is retrieved from positions.
			// As a result, we skip them here.
		} else if !strings.HasPrefix(lockedCoin.Denom, concentratedSharePrefix) {
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
func (p *passthroughUseCase) getBankBalances(ctx context.Context, address string) (sdk.Coins, error) {
	allBalances, err := p.passthroughGRPCClient.AllBalances(ctx, address)
	if err != nil {
		// This error is not expected and is considered fatal
		// To be handled as needed by the caller.
		return nil, err
	}

	coins := sdk.Coins{}

	for _, balance := range allBalances {
		if strings.HasPrefix(balance.Denom, gammSharePrefix) {
			exitCoins, err := p.handleGammShares(balance)
			if err != nil {
				p.logger.Error("error converting gamm share from balances to underlying coins", zap.Error(err))
				continue
			}

			coins = coins.Add(exitCoins...)
		} else {
			// Performance optimization to avoid sorting with Add(...)
			coins = append(coins, balance)
		}
	}

	// Sort since we appended without sorting in the loop
	return coins.Sort(), nil
}

// handleGammShares converts GAMM shares to underlying coins
// Returns error if fails to convert GAMM shares to underlying coins.
// Returns the underlying coins if successful.
// CONTRACT: coin is a gamm share
func (p *passthroughUseCase) handleGammShares(coin sdk.Coin) (sdk.Coins, error) {
	// calc underlying coins from gamm shares
	splitDenom := strings.Split(coin.Denom, denomShareSeparator)
	poolID := splitDenom[len(splitDenom)-1]
	poolIDInt, err := strconv.ParseUint(poolID, 10, 64)
	if err != nil {
		return sdk.Coins{}, err
	}

	exitCoins, err := p.poolsUseCase.CalcExitCFMMPool(poolIDInt, coin.Amount)
	if err != nil {
		return sdk.Coins{}, err
	}

	return exitCoins, nil
}
