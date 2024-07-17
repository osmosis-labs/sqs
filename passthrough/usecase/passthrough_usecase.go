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

	// TODO: set in constructor
	priceGetter           mvc.PriceGetter
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
func NewPassThroughUsecase(passthroughGRPCClient passthroughdomain.PassthroughGRPCClient, puc mvc.PoolsUsecase, priceGetter mvc.PriceGetter, liquidityPricer domain.LiquidityPricer, defaultQuoteDenom string, logger log.Logger) *passthroughUseCase {
	return &passthroughUseCase{
		poolsUseCase: puc,

		passthroughGRPCClient: passthroughGRPCClient,

		priceGetter:       priceGetter,
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

	accountCoinsResult, capitalizationTotal, err := p.instrumentCoinsWithPrices(ctx, coins)
	if err != nil {
		p.logger.Error("error instrumenting coins with prices", zap.Error(err))
		finalErr = err
	}

	return passthroughdomain.PortfolioAssetsResult{
		AccountCoinsResult: accountCoinsResult,
		TotalValueCap:      capitalizationTotal,
	}, finalErr // Note that we skip all errors for best-effort aggregation but propagate the last one observed, if any.
}

func (p *passthroughUseCase) instrumentCoinsWithPrices(ctx context.Context, coins sdk.Coins) ([]passthroughdomain.AccountCoinsResult, osmomath.Dec, error) {
	coinDenoms := coins.Denoms()

	// Compute prices for the final coins
	priceResult, err := p.priceGetter.GetPrices(ctx, coinDenoms, []string{p.defaultQuoteDenom}, domain.ChainPricingSourceType)
	if err != nil {
		return nil, osmomath.Dec{}, err
	}

	// Instrument coins with prices
	coinsWithPrices := make([]passthroughdomain.AccountCoinsResult, 0, len(coins))
	capitalizaionTotal := osmomath.ZeroDec()

	for _, coin := range coins {
		price := priceResult.GetPriceForDenom(coin.Denom, p.defaultQuoteDenom)

		coinCapitalization := p.liquidityPricer.PriceCoin(coin, price)

		capitalizaionTotal = capitalizaionTotal.Add(coinCapitalization)

		coinsWithPrices = append(coinsWithPrices, passthroughdomain.AccountCoinsResult{
			Coin:                coin,
			CapitalizationValue: coinCapitalization,
		})
	}

	return coinsWithPrices, capitalizaionTotal, nil
}

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
			splitDenom := strings.Split(lockedCoin.Denom, "/")
			poolID := splitDenom[len(splitDenom)-1]
			poolIDInt, err := strconv.ParseInt(poolID, 10, 64)
			if err != nil {
				return nil, err
			}

			exitCoins, err := p.poolsUseCase.CalcExitCFMMPool(uint64(poolIDInt), lockedCoin.Amount)
			if err != nil {
				return nil, err
			}
			coins = coins.Add(exitCoins...)
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
			// calc underlying coins from gamm shares
			splitDenom := strings.Split(balance.Denom, denomShareSeparator)
			poolID := splitDenom[len(splitDenom)-1]
			poolIDInt, err := strconv.ParseUint(poolID, 10, 64)
			if err != nil {
				p.logger.Error("failed to parse pool id when retrieving bank balances", zap.Uint64("pool_id", poolIDInt), zap.Error(err))
				// Skip unexpected error silently.
				continue
			}

			exitCoins, err := p.poolsUseCase.CalcExitCFMMPool(poolIDInt, balance.Amount)
			if err != nil {
				p.logger.Error("failed to calculate exit coins from pool", zap.Uint64("pool_id", poolIDInt), zap.Error(err))
				// Skip unexpected error silently.
				continue
			}
			coins = coins.Add(exitCoins...)
		} else {
			coins = coins.Add(balance)
		}
	}

	return coins, nil
}
