package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	sdkmath "math"

	"cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsutil/datafetchers"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	sqspassthroughdomain "github.com/osmosis-labs/sqs/sqsdomain/passthroughdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	cosmwasmpoolmodel "github.com/osmosis-labs/osmosis/v27/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v27/x/gamm/types"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v27/x/poolmanager/types"

	errorsmod "cosmossdk.io/errors"
)

type orderBookEntry struct {
	PoolID          uint64
	LiquidityCap    osmomath.Int
	ContractAddress string
}

type poolsUseCase struct {
	pools            sync.Map
	routerRepository routerrepo.RouterRepository

	canonicalOrderBookForBaseQuoteDenom sync.Map
	canonicalOrderbookPoolIDs           sync.Map

	cosmWasmPoolsParams cosmwasmdomain.CosmWasmPoolsParams

	aprPrefetcher      datafetchers.MapFetcher[uint64, sqspassthroughdomain.PoolAPR]
	poolFeesPrefetcher datafetchers.MapFetcher[uint64, sqspassthroughdomain.PoolFee]

	logger log.Logger
}

var _ mvc.PoolsUsecase = &poolsUseCase{}

const (
	// baseQuoteKeySeparator is the separator used to separate base and quote denom in the key.
	baseQuoteKeySeparator = "~"
)

// NewPoolsUsecase will create a new pools use case object
func NewPoolsUsecase(poolsConfig *domain.PoolsConfig, chainGRPCGatewayEndpoint string, routerRepository routerrepo.RouterRepository, scalingFactorGetterCb domain.ScalingFactorGetterCb, logger log.Logger) (*poolsUseCase, error) {
	transmuterCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.TransmuterCodeIDs))
	for _, codeID := range poolsConfig.TransmuterCodeIDs {
		transmuterCodeIDsMap[codeID] = struct{}{}
	}

	alloyedTransmuterCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.AlloyedTransmuterCodeIDs))
	for _, codeId := range poolsConfig.AlloyedTransmuterCodeIDs {
		alloyedTransmuterCodeIDsMap[codeId] = struct{}{}
	}

	orderbookCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.OrderbookCodeIDs))
	for _, codeId := range poolsConfig.OrderbookCodeIDs {
		orderbookCodeIDsMap[codeId] = struct{}{}
	}

	generalizedCosmWasmCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.GeneralCosmWasmCodeIDs))
	for _, codeID := range poolsConfig.GeneralCosmWasmCodeIDs {
		generalizedCosmWasmCodeIDsMap[codeID] = struct{}{}
	}

	wasmClient, err := initializeWasmClient(chainGRPCGatewayEndpoint)
	if err != nil {
		return nil, err
	}

	return &poolsUseCase{
		pools:            sync.Map{},
		routerRepository: routerRepository,

		cosmWasmPoolsParams: cosmwasmdomain.CosmWasmPoolsParams{
			Config: domain.CosmWasmPoolRouterConfig{
				TransmuterCodeIDs:        transmuterCodeIDsMap,
				AlloyedTransmuterCodeIDs: alloyedTransmuterCodeIDsMap,
				OrderbookCodeIDs:         orderbookCodeIDsMap,
				GeneralCosmWasmCodeIDs:   generalizedCosmWasmCodeIDsMap,
				ChainGRPCGatewayEndpoint: chainGRPCGatewayEndpoint,
			},

			WasmClient: wasmClient,

			ScalingFactorGetterCb: scalingFactorGetterCb,
		},

		logger: logger,
	}, nil
}

// GetAllPools returns all pools from the repository.
func (p *poolsUseCase) GetAllPools() (pools []sqsdomain.PoolI, err error) {
	p.pools.Range(func(key, value interface{}) bool {
		pool, ok := value.(sqsdomain.PoolI)
		if !ok {
			err = fmt.Errorf("failed to cast pool with value %v", value)
			return false
		}

		pools = append(pools, pool)
		return true
	})

	return pools, nil
}

// GetRoutesFromCandidates implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetRoutesFromCandidates(candidateRoutes sqsdomain.CandidateRoutes, tokenInDenom, tokenOutDenom string) ([]route.RouteImpl, error) {
	// We track whether a route contains a generalized cosmwasm pool
	// so that we can exclude it from split quote logic.
	// The reason for this is that making network requests to chain is expensive.
	// As a result, we want to minimize the number of requests we make.
	containsGeneralizedCosmWasmPool := false

	// Convert each candidate route into the actual route with all pool data
	routes := make([]route.RouteImpl, 0, len(candidateRoutes.Routes))
	for _, candidateRoute := range candidateRoutes.Routes {
		previousTokenOutDenom := tokenInDenom

		routablePools := make([]domain.RoutablePool, 0, len(candidateRoute.Pools))

		// For fault tolerance, instead of bubbling up the error and skipping an entire
		// request, we should detect the error and skip the route.
		skipErrorRoute := false

		for _, candidatePool := range candidateRoute.Pools {
			pool, err := p.GetPool(candidatePool.ID)
			if err != nil {
				return nil, err
			}

			// Get taker fee
			takerFee, exists := p.routerRepository.GetTakerFee(previousTokenOutDenom, candidatePool.TokenOutDenom)
			if !exists {
				takerFee = sqsdomain.DefaultTakerFee
			}

			routablePool, err := pools.NewRoutablePool(pool, candidatePool.TokenOutDenom, takerFee, p.cosmWasmPoolsParams)
			if err != nil {
				skipErrorRoute = true
				break
			}

			isGeneralizedCosmWasmPool := routablePool.GetSQSType() == domain.GeneralizedCosmWasm
			if isGeneralizedCosmWasmPool {
				containsGeneralizedCosmWasmPool = true
			}

			// Create routable pool
			routablePools = append(routablePools, routablePool)
		}

		// Skip the route if there was an error
		if skipErrorRoute {
			continue
		}

		routes = append(routes, route.RouteImpl{
			Pools:                      routablePools,
			HasGeneralizedCosmWasmPool: containsGeneralizedCosmWasmPool,
			HasCanonicalOrderbookPool:  candidateRoute.IsCanonicalOrderboolRoute,
		})
	}

	return routes, nil
}

// GetTickModelMap implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetTickModelMap(poolIDs []uint64) (map[uint64]*sqsdomain.TickModel, error) {
	tickModelMap := make(map[uint64]*sqsdomain.TickModel, len(poolIDs))
	for _, poolID := range poolIDs {
		pool, err := p.GetPool(poolID)
		if err != nil {
			return nil, err
		}

		if pool.GetType() != poolmanagertypes.Concentrated {
			return nil, fmt.Errorf("pool with ID %d is not concentrated", poolID)
		}

		poolWrapper, ok := pool.(*sqsdomain.PoolWrapper)
		if !ok {
			return nil, domain.ConcentratedTickModelNotSetError{
				PoolId: poolID,
			}
		}

		tickModelMap[poolID] = poolWrapper.TickModel
	}

	return tickModelMap, nil
}

// GetPool implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetPool(poolID uint64) (sqsdomain.PoolI, error) {
	poolObj, ok := p.pools.Load(poolID)
	if !ok {
		return nil, domain.PoolNotFoundError{PoolID: poolID}
	}

	pool, ok := poolObj.(sqsdomain.PoolI)
	if !ok {
		return nil, fmt.Errorf("failed to cast pool with ID %d", poolID)
	}

	return pool, nil
}

// GetPoolSpotPrice implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetPoolSpotPrice(ctx context.Context, poolID uint64, takerFee math.LegacyDec, quoteAsset, baseAsset string) (osmomath.BigDec, error) {
	pool, err := p.GetPool(poolID)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Instrument pool with tick model data if concentrated
	if err := p.getTicksAndSetTickModelIfConcentrated(pool); err != nil {
		return osmomath.BigDec{}, err
	}

	// N.B.: Empty string for token out denom because it is irrelevant for calculating spot price.
	// It is only relevant in the context of routing
	routablePool, err := pools.NewRoutablePool(pool, "", takerFee, p.cosmWasmPoolsParams)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return routablePool.CalcSpotPrice(ctx, baseAsset, quoteAsset)
}

// IsGeneralCosmWasmCodeID implements mvc.PoolsUsecase.
func (p *poolsUseCase) IsGeneralCosmWasmCodeID(codeId uint64) bool {
	_, isGenneralCosmWasmCodeID := p.cosmWasmPoolsParams.Config.GeneralCosmWasmCodeIDs[codeId]
	return isGenneralCosmWasmCodeID
}

// setTickModelMapIfConcentrated sets tick model for concentrated pools. No-op if pool is not concentrated.
// If the pool is concentrated but the map does not contains the tick model, an error is returned.
// The input pool parameter is mutated.
func setTickModelIfConcentrated(pool sqsdomain.PoolI, tickModelMap map[uint64]*sqsdomain.TickModel) error {
	if pool.GetType() == poolmanagertypes.Concentrated {
		// Get tick model for concentrated pool
		tickModel, ok := tickModelMap[pool.GetId()]
		if !ok {
			return domain.ConcentratedTickModelNotSetError{
				PoolId: pool.GetId(),
			}
		}

		if err := pool.SetTickModel(tickModel); err != nil {
			return err
		}
	}

	return nil
}

// getTicksAndSetTickModelIfConcentrated gets tick model for concentrated pools and sets it if this is a concentrated pool.
// The input pool parameter is mutated.
// No-op if pool is not concentrated.
func (p *poolsUseCase) getTicksAndSetTickModelIfConcentrated(pool sqsdomain.PoolI) error {
	if pool.GetType() == poolmanagertypes.Concentrated {
		// Get tick model for concentrated pools
		tickModelMap, err := p.GetTickModelMap([]uint64{pool.GetId()})
		if err != nil {
			return err
		}

		// Set tick model for concentrated pools
		if err := setTickModelIfConcentrated(pool, tickModelMap); err != nil {
			return err
		}
	}

	return nil
}

// GetPools implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetPools(opts ...domain.PoolsOption) ([]sqsdomain.PoolI, error) {
	options := domain.PoolsOptions{
		MinPoolLiquidityCap:  0,
		PoolIDFilter:         []uint64{},
		WithMarketIncentives: false,
		HadEmptyFilter:       false,
	}

	for _, opt := range opts {
		opt(&options)
	}

	if options.HadEmptyFilter {
		return nil, nil
	}

	var (
		pools []sqsdomain.PoolI
	)

	if len(options.PoolIDFilter) > 0 {
		// Get specific pools
		pools = make([]sqsdomain.PoolI, 0, len(options.PoolIDFilter))
		for _, poolID := range options.PoolIDFilter {
			pool, err := p.GetPool(poolID)
			if err != nil {
				return nil, err
			}

			// Add the pool to pools if it matches the options
			pools = p.retainPoolIfMatchesOptions(pools, pool, options)
		}
	} else {
		// Pre-allocate 2000 since this is how many pools there are today.
		pools = make([]sqsdomain.PoolI, 0, 2000)
		p.pools.Range(func(key, value interface{}) bool {
			pool, ok := value.(sqsdomain.PoolI)
			// Check filter is non-zero to avoid more expensive get liquidity cap check.
			if ok {
				// Add the pool to pools if it matches the options
				pools = p.retainPoolIfMatchesOptions(pools, pool, options)
			}
			return true
		})
	}

	return pools, nil
}

// StorePools implements mvc.PoolsUsecase.
func (p *poolsUseCase) StorePools(pools []sqsdomain.PoolI) error {
	for _, pool := range pools {
		// Store pool
		poolID := pool.GetId()
		p.pools.Store(poolID, pool)

		// If orderbook, update top liquidity pool for base and quote denom if it has higher liquidity capitalization.
		sqsModel := pool.GetSQSPoolModel()
		cosmWasmPoolModel := sqsModel.CosmWasmPoolModel
		if cosmWasmPoolModel != nil && cosmWasmPoolModel.Data.Orderbook != nil && cosmWasmPoolModel.IsOrderbook() {
			baseDenom := cosmWasmPoolModel.Data.Orderbook.BaseDenom
			quoteDenom := cosmWasmPoolModel.Data.Orderbook.QuoteDenom
			poolLiquidityCapitalization := pool.GetLiquidityCap()

			// Get contract address from chain pool
			chainPool := pool.GetUnderlyingPool()
			chainCosmWasmPool, ok := chainPool.(*cosmwasmpoolmodel.CosmWasmPool)
			if !ok || chainCosmWasmPool == nil {
				p.logger.Error("failed to cast chain pool to CosmWasmPool", zap.Uint64("poolID", poolID))
				continue
			}
			contractAddress := chainCosmWasmPool.ContractAddress

			// Process orderbook pool ID for base and quote denom
			_, err := p.processOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom, poolID, poolLiquidityCapitalization, contractAddress)
			if err != nil {
				p.logger.Error(err.Error())
				// Continue to the next pool
				continue
			}
		}
	}
	return nil
}

// processOrderbookPoolIDForBaseQuote processes the orderbook pool ID for the base and quote denom and pool liquidity
// capitalization. If the current pool has higher liquidity capitalization than the top liquidity pool, update the top liquidity pool
// for the given base and quote denom.
// Returns true if the top liquidity pool is updated, false otherwise.
// Returns an error if the previous top orderbook entry cannot be casted to the right type.
// CONTRACT: the given poolID is an orderbook pool.
func (p *poolsUseCase) processOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom string, poolID uint64, poolLiquidityCapitalization osmomath.Int, contractAddress string) (updatedBool bool, err error) {
	// Format base and quote denom key.
	baseQuoteKey := formatBaseQuoteDenom(baseDenom, quoteDenom)

	// Determine there is an existing top liquidity pool for the base and quote denom.
	topLiquidityOrderBook, found := p.canonicalOrderBookForBaseQuoteDenom.Load(baseQuoteKey)
	if found {
		// Cast to orderBookEntry
		topLiquidityOrderBookEntry, ok := topLiquidityOrderBook.(orderBookEntry)
		if !ok {
			err = domain.FailCastCanonicalOrderbookEntryError{
				BaseQuoteKey: baseQuoteKey,
			}
			return false, err
		}

		// If the current pool has lower or equak liquidity capitalization than the top liquidity pool
		// continue to the next pool
		if poolLiquidityCapitalization.LTE(topLiquidityOrderBookEntry.LiquidityCap) {
			return false, nil
		}

		// Remove the old pool from the canonical map
		p.canonicalOrderbookPoolIDs.Delete(topLiquidityOrderBookEntry.PoolID)
	}

	// If not found or the current pool has higher liquidity capitalization than the top liquidity pool
	// update the top liquidity pool
	p.canonicalOrderBookForBaseQuoteDenom.Store(baseQuoteKey, orderBookEntry{
		PoolID:          poolID,
		LiquidityCap:    poolLiquidityCapitalization,
		ContractAddress: contractAddress,
	})

	// Store the pool ID in the canonical orderbook pool IDs
	p.canonicalOrderbookPoolIDs.Store(poolID, struct{}{})

	return true, nil
}

// GetCanonicalOrderbookPool implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetCanonicalOrderbookPool(baseDenom, quoteDenom string) (uint64, string, error) {
	baseQuote := formatBaseQuoteDenom(baseDenom, quoteDenom)
	topLiquidityOrderBook, found := p.canonicalOrderBookForBaseQuoteDenom.Load(baseQuote)
	if !found {
		return 0, "", fmt.Errorf("canonical orderbook not found for base %s and quote %s", baseDenom, quoteDenom)
	}

	topLiquidityOrderBookEntry, ok := topLiquidityOrderBook.(orderBookEntry)
	if !ok {
		return 0, "", fmt.Errorf("failed to cast orderbook entry with value %v", topLiquidityOrderBook)
	}

	return topLiquidityOrderBookEntry.PoolID, topLiquidityOrderBookEntry.ContractAddress, nil
}

// GetAllCanonicalOrderbookPoolIDs implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetAllCanonicalOrderbookPoolIDs() ([]domain.CanonicalOrderBooksResult, error) {
	var (
		results []domain.CanonicalOrderBooksResult
		err     error
	)

	p.canonicalOrderBookForBaseQuoteDenom.Range(func(key, value any) bool {
		// Cast key to string
		baseQuoteKey, ok := key.(string)
		if !ok {
			err = domain.FailCastCanonicalOrderbookKeyError{
				BaseQuoteKey: baseQuoteKey,
			}
			return false
		}

		// split base and quote denom
		denoms := strings.Split(baseQuoteKey, baseQuoteKeySeparator)
		if len(denoms) != 2 {
			err = domain.FailSplitCanonicalOrderBookKeyError{
				BaseQuoteKey: baseQuoteKey,
			}
			return false
		}

		baseDenom := denoms[0]
		quoteDenom := denoms[1]

		// Cast value to orderBookEntry
		topLiquidityOrderBook, ok := value.(orderBookEntry)
		if !ok {
			err = domain.FailCastCanonicalOrderbookEntryError{
				BaseQuoteKey: baseQuoteKey,
			}
			return false
		}

		results = append(results, domain.CanonicalOrderBooksResult{
			Base:            baseDenom,
			Quote:           quoteDenom,
			PoolID:          topLiquidityOrderBook.PoolID,
			ContractAddress: topLiquidityOrderBook.ContractAddress,
		})

		return true
	})

	// Sort by pool ID for deterministic results
	sort.Slice(results, func(i, j int) bool {
		return results[i].PoolID < results[j].PoolID
	})

	return results, err
}

// RegisterAPRFetcher registers the APR fetcher for the passthrough use case.
func (p *poolsUseCase) RegisterAPRFetcher(aprFetcher datafetchers.MapFetcher[uint64, sqspassthroughdomain.PoolAPR]) {
	p.aprPrefetcher = aprFetcher
}

// RegisterPoolFeesFetcher registers the pool fees fetcher for the passthrough use case.
func (p *poolsUseCase) RegisterPoolFeesFetcher(poolFeesFetcher datafetchers.MapFetcher[uint64, sqspassthroughdomain.PoolFee]) {
	p.poolFeesPrefetcher = poolFeesFetcher
}

// IsCanonicalOrderbookPool implements mvc.PoolsUsecase.
func (p *poolsUseCase) IsCanonicalOrderbookPool(poolID uint64) bool {
	_, exists := p.canonicalOrderbookPoolIDs.Load(poolID)
	return exists
}

// GetCosmWasmPoolConfig implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetCosmWasmPoolConfig() domain.CosmWasmPoolRouterConfig {
	return p.cosmWasmPoolsParams.Config
}

// CalcExitCFMMPool implements mvc.PoolsUsecase.
func (p *poolsUseCase) CalcExitCFMMPool(poolID uint64, exitingSharesIn osmomath.Int) (sdk.Coins, error) {
	sqsPool, err := p.GetPool(poolID)
	if err != nil {
		return nil, err
	}

	if sqsPool.GetType() != poolmanagertypes.Balancer && sqsPool.GetType() != poolmanagertypes.Stableswap {
		return nil, fmt.Errorf("invalid pool type for pool ID %d, expected CFMM pool", poolID)
	}

	pool, ok := sqsPool.GetUnderlyingPool().(types.CFMMPoolI)
	if !ok {
		return nil, fmt.Errorf("failed to cast underlying pool to CFMMPoolI for ID: %d", poolID)
	}

	// fine to pass empty context as no data is mutated
	exitFee := pool.GetExitFee(sdk.Context{})

	return calcExitPool(sdk.Context{}, pool, exitingSharesIn, exitFee)
}

// errMsgFormatSharesLargerThanMax is the error message format for when the exiting shares are larger than the max allowed.
const errMsgFormatSharesLargerThanMax = "cannot exit all shares in a pool. Attempted to exit %f shares, max allowed is %f"

// calcExitPool is a helper function to calculate the exit pool.
// It is a direct port of the CalcExitPool function from the Gamm module with some modifications
// to optimize the calculation for performance.
// @link https://github.com/osmosis-labs/osmosis/blob/fde1776476d9c2f849dcbfb30ca3ec64d0e12863/x/gamm/pool-models/internal/cfmm_common/lp.go#L18
func calcExitPool(ctx sdk.Context, pool types.CFMMPoolI, exitingSharesIn osmomath.Int, exitFee osmomath.Dec) (sdk.Coins, error) {
	totalShares, err := strconv.ParseFloat(pool.GetTotalShares().String(), 64)
	if err != nil {
		return sdk.Coins{}, err
	}

	exitingShares, err := strconv.ParseFloat(exitingSharesIn.String(), 64)
	if err != nil {
		return sdk.Coins{}, err
	}

	if exitingShares >= totalShares {
		return sdk.Coins{}, errorsmod.Wrapf(types.ErrLimitMaxAmount, errMsgFormatSharesLargerThanMax, exitingShares, totalShares-float64(osmomath.OneInt().Int64()))
	}

	// refundedShares = exitingShares * (1 - exit fee)
	// with 0 exit fee optimization
	var refundedShares float64
	if !exitFee.IsZero() {
		f, err := exitFee.Float64()
		if err != nil {
			return sdk.Coins{}, err
		}

		// exitingShares * (1 - exit fee)
		oneSubExitFee := 1.0 - f
		refundedShares = exitingShares * oneSubExitFee
	} else {
		refundedShares = exitingShares
	}

	shareOutRatio := refundedShares / totalShares

	// exitedCoins = shareOutRatio * pool liquidity
	poolLiquidity := pool.GetTotalPoolLiquidity(ctx)
	exitedCoins := make(sdk.Coins, 0, len(poolLiquidity))

	for _, asset := range poolLiquidity {
		// round down here, due to not wanting to over-exit
		amount, err := strconv.ParseFloat(asset.Amount.String(), 64)
		if err != nil {
			return sdk.Coins{}, err
		}

		exitAmt := shareOutRatio * amount
		if exitAmt <= 0 {
			continue
		}

		if exitAmt >= amount {
			return sdk.Coins{}, errors.New("too many shares out")
		}

		// If the exit amount is within 1e-9 of an integer, round it to the nearest integer
		if sdkmath.Abs(exitAmt-sdkmath.Round(exitAmt)) < 1e-9 {
			exitAmt = (sdkmath.Round(exitAmt))
		}

		exitedCoins = append(exitedCoins, sdk.Coin{
			Denom:  asset.Denom,
			Amount: math.NewInt(int64(exitAmt)),
		})
	}

	return exitedCoins, nil
}

// retainPoolIfMatchesOptions retains the pool if it matches the options.
// Returns the updated pools.
// The input poolConsidered parameter is mutated with options if options specify to set APR and fee data.
// The input poolsToUpdate parameter is mutated with the poolConsidered if it matches the options.
func (p *poolsUseCase) retainPoolIfMatchesOptions(poolsToUpdate []sqsdomain.PoolI, poolConsidered sqsdomain.PoolI, options domain.PoolsOptions) []sqsdomain.PoolI {
	if options.MinPoolLiquidityCap == 0 || poolConsidered.GetLiquidityCap().Uint64() >= options.MinPoolLiquidityCap {
		// Set APR and fee data if configured
		p.setPoolAPRAndFeeDataIfConfigured(poolConsidered, options)

		poolsToUpdate = append(poolsToUpdate, poolConsidered)
	}
	return poolsToUpdate
}

// setPoolAPRAndFeeDataIfConfigured sets the APR and fee data for the pool if the options are configured.
// No-op otherwise.
// Logs an error if fails to get APR or pool fee data.
// The input pool parameter is mutated.
// The input options parameter is used to determine whether to set APR and fee data.
func (p *poolsUseCase) setPoolAPRAndFeeDataIfConfigured(pool sqsdomain.PoolI, options domain.PoolsOptions) {
	if options.WithMarketIncentives {
		poolID := pool.GetId()

		// Get APR data
		poolAPRData, _, isStale, err := p.aprPrefetcher.GetByKey(poolID)
		if err != nil {
			// Log error if fails to get APR data
			p.logger.Error("failed to get APR data", zap.Uint64("poolID", poolID), zap.Error(err))
		}

		// Set APR data
		pool.SetAPRData(sqspassthroughdomain.PoolAPRDataStatusWrap{
			PoolAPR: poolAPRData,
			IsStale: isStale,
			IsError: err != nil,
		})

		// Get pool fee data
		poolFeeData, _, isStale, err := p.poolFeesPrefetcher.GetByKey(poolID)
		if err != nil {
			// Log error if fails to get pool fee data
			p.logger.Error("failed to get pool fee data", zap.Uint64("poolID", poolID), zap.Error(err))
		}

		// Set pool fee data
		pool.SetFeesData(sqspassthroughdomain.PoolFeesDataStatusWrap{
			PoolFee: poolFeeData,
			IsStale: isStale,
			IsError: err != nil,
		})
	}
}

// formatBaseQuoteDenom formats the base and quote denom into a single string with a separator.
func formatBaseQuoteDenom(baseDenom, quoteDenom string) string {
	return baseDenom + baseQuoteKeySeparator + quoteDenom
}

// initializeWasmClient initializes the wasm client given the node URI
// Returns error if fails to initialize the client
func initializeWasmClient(grpcGatewayEndpoint string) (wasmtypes.QueryClient, error) {
	grpcClient, err := grpc.NewClient(grpcGatewayEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, err
	}

	wasmClient := wasmtypes.NewQueryClient(grpcClient)

	return wasmClient, nil
}
