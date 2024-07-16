package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

type orderBookEntry struct {
	PoolID       uint64
	LiquidityCap osmomath.Int
}

type poolsUseCase struct {
	pools            sync.Map
	routerRepository routerrepo.RouterRepository
	cosmWasmConfig   domain.CosmWasmPoolRouterConfig

	canonicalOrderBookForBaseQuoteDenom sync.Map

	scalingFactorGetterCb domain.ScalingFactorGetterCb

	logger log.Logger
}

var _ mvc.PoolsUsecase = &poolsUseCase{}

const (
	// baseQuoteKeySeparator is the separator used to separate base and quote denom in the key.
	baseQuoteKeySeparator = "~"
)

// NewPoolsUsecase will create a new pools use case object
func NewPoolsUsecase(poolsConfig *domain.PoolsConfig, chainGRPCGatewayEndpoint string, routerRepository routerrepo.RouterRepository, scalingFactorGetterCb domain.ScalingFactorGetterCb, logger log.Logger) *poolsUseCase {
	transmuterCodeIDsMap := make(map[uint64]struct{}, len(poolsConfig.TransmuterCodeIDs))
	for _, codeId := range poolsConfig.TransmuterCodeIDs {
		transmuterCodeIDsMap[codeId] = struct{}{}
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
	for _, codeId := range poolsConfig.GeneralCosmWasmCodeIDs {
		generalizedCosmWasmCodeIDsMap[codeId] = struct{}{}
	}

	return &poolsUseCase{
		cosmWasmConfig: domain.CosmWasmPoolRouterConfig{
			TransmuterCodeIDs:        transmuterCodeIDsMap,
			AlloyedTransmuterCodeIDs: alloyedTransmuterCodeIDsMap,
			OrderbookCodeIDs:         orderbookCodeIDsMap,
			GeneralCosmWasmCodeIDs:   generalizedCosmWasmCodeIDsMap,
			ChainGRPCGatewayEndpoint: chainGRPCGatewayEndpoint,
		},

		pools:                 sync.Map{},
		routerRepository:      routerRepository,
		scalingFactorGetterCb: scalingFactorGetterCb,

		logger: logger,
	}
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

			routablePool, err := pools.NewRoutablePool(pool, candidatePool.TokenOutDenom, takerFee, p.cosmWasmConfig, p.scalingFactorGetterCb)
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
	routablePool, err := pools.NewRoutablePool(pool, "", takerFee, p.cosmWasmConfig, p.scalingFactorGetterCb)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return routablePool.CalcSpotPrice(ctx, baseAsset, quoteAsset)
}

// IsGeneralCosmWasmCodeID implements mvc.PoolsUsecase.
func (p *poolsUseCase) IsGeneralCosmWasmCodeID(codeId uint64) bool {
	_, isGenneralCosmWasmCodeID := p.cosmWasmConfig.GeneralCosmWasmCodeIDs[codeId]
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
func (p *poolsUseCase) GetPools(poolIDs []uint64) ([]sqsdomain.PoolI, error) {
	pools := make([]sqsdomain.PoolI, 0, len(poolIDs))

	for _, poolID := range poolIDs {
		pool, err := p.GetPool(poolID)
		if err != nil {
			return nil, err
		}

		pools = append(pools, pool)
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
		if cosmWasmPoolModel != nil && cosmWasmPoolModel.IsOrderbook() {
			baseDenom := cosmWasmPoolModel.Data.Orderbook.BaseDenom
			quoteDenom := cosmWasmPoolModel.Data.Orderbook.QuoteDenom
			poolLiquidityCapitalization := pool.GetLiquidityCap()

			// Process orderbook pool ID for base and quote denom
			_, err := p.processOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom, poolID, poolLiquidityCapitalization)
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
func (p *poolsUseCase) processOrderbookPoolIDForBaseQuote(baseDenom, quoteDenom string, poolID uint64, poolLiquidityCapitalization osmomath.Int) (updatedBool bool, err error) {
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
	}

	// If not found or the current pool has higher liquidity capitalization than the top liquidity pool
	// update the top liquidity pool
	p.canonicalOrderBookForBaseQuoteDenom.Store(baseQuoteKey, orderBookEntry{
		PoolID:       poolID,
		LiquidityCap: poolLiquidityCapitalization,
	})

	return true, nil
}

// GetCanonicalOrderbookPoolID implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetCanonicalOrderbookPoolID(baseDenom, quoteDenom string) (uint64, error) {
	baseQuote := formatBaseQuoteDenom(baseDenom, quoteDenom)
	topLiquidityOrderBook, found := p.canonicalOrderBookForBaseQuoteDenom.Load(baseQuote)
	if !found {
		return 0, fmt.Errorf("canonical orderbook not found for base %s and quote %s", baseDenom, quoteDenom)
	}

	topLiquidityOrderBookEntry, ok := topLiquidityOrderBook.(orderBookEntry)
	if !ok {
		return 0, fmt.Errorf("failed to cast orderbook entry with value %v", topLiquidityOrderBook)
	}

	return topLiquidityOrderBookEntry.PoolID, nil
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
			Base:   baseDenom,
			Quote:  quoteDenom,
			PoolID: topLiquidityOrderBook.PoolID,
		})

		return true
	})

	// Sort by pool ID for deterministic results
	sort.Slice(results, func(i, j int) bool {
		return results[i].PoolID < results[j].PoolID
	})

	return results, err
}

// GetCosmWasmPoolConfig implements mvc.PoolsUsecase.
func (p *poolsUseCase) GetCosmWasmPoolConfig() domain.CosmWasmPoolRouterConfig {
	return p.cosmWasmConfig
}

// formatBaseQuoteDenom formats the base and quote denom into a single string with a separator.
func formatBaseQuoteDenom(baseDenom, quoteDenom string) string {
	return baseDenom + baseQuoteKeySeparator + quoteDenom
}
