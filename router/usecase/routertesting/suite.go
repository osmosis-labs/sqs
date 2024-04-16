package routertesting

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/domain/mvc"
	ingestusecase "github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/log"
	poolsusecase "github.com/osmosis-labs/sqs/pools/usecase"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"
	"github.com/osmosis-labs/sqs/sqsdomain"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v24/app"
	"github.com/osmosis-labs/osmosis/v24/app/apptesting"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v24/x/poolmanager/types"
)

type RouterTestHelper struct {
	apptesting.ConcentratedKeeperTestHelper
}

// Mock mainnet state
type MockMainnetState struct {
	TickMap        map[uint64]*sqsdomain.TickModel
	TakerFeeMap    sqsdomain.TakerFeeMap
	TokensMetadata map[string]domain.Token
	PricingConfig  domain.PricingConfig
}

type MockMainnetUsecase struct {
	Pools  mvc.PoolsUsecase
	Router mvc.RouterUsecase
	Tokens mvc.TokensUsecase
	Ingest mvc.IngestUsecase
}

const (
	DefaultPoolID = uint64(1)

	relativePathMainnetFiles = "/router/usecase/routertesting/parsing/"
	poolsFileName            = "pools.json"
	takerFeesFileName        = "taker_fees.json"
	tokensMetadataFileName   = "tokens.json"
)

var (
	// Concentrated liquidity constants
	Denom0 = ETH
	Denom1 = USDC

	DefaultCurrentTick = apptesting.DefaultCurrTick

	DefaultAmt0 = apptesting.DefaultAmt0
	DefaultAmt1 = apptesting.DefaultAmt1

	DefaultCoin0 = apptesting.DefaultCoin0
	DefaultCoin1 = apptesting.DefaultCoin1

	DefaultLiquidityAmt = apptesting.DefaultLiquidityAmt

	// router specific variables
	DefaultTickModel = &sqsdomain.TickModel{
		Ticks:            []sqsdomain.LiquidityDepthsWithRange{},
		CurrentTickIndex: 0,
		HasNoLiquidity:   false,
	}

	NoTakerFee = osmomath.ZeroDec()

	DefaultTakerFee     = osmomath.MustNewDecFromStr("0.002")
	DefaultPoolBalances = sdk.NewCoins(
		sdk.NewCoin(DenomOne, DefaultAmt0),
		sdk.NewCoin(DenomTwo, DefaultAmt1),
	)
	DefaultSpreadFactor = osmomath.MustNewDecFromStr("0.005")

	DefaultPool = &mocks.MockRoutablePool{
		ID:                   DefaultPoolID,
		Denoms:               []string{DenomOne, DenomTwo},
		TotalValueLockedUSDC: osmomath.NewInt(10),
		PoolType:             poolmanagertypes.Balancer,
		Balances:             DefaultPoolBalances,
		TakerFee:             DefaultTakerFee,
		SpreadFactor:         DefaultSpreadFactor,
	}
	EmptyRoute                   = route.RouteImpl{}
	EmpyCosmWasmPoolRouterConfig = domain.CosmWasmPoolRouterConfig{
		TransmuterCodeIDs:      map[uint64]struct{}{},
		GeneralCosmWasmCodeIDs: map[uint64]struct{}{},
		NodeURI:                "",
	}

	// Test denoms
	DenomOne   = denomNum(1)
	DenomTwo   = denomNum(2)
	DenomThree = denomNum(3)
	DenomFour  = denomNum(4)
	DenomFive  = denomNum(5)
	DenomSix   = denomNum(6)

	UOSMO   = "uosmo"
	ATOM    = "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2"
	STOSMO  = "ibc/D176154B0C63D1F9C6DCFB4F70349EBF2E2B5A87A05902F57A6AE92B863E9AEC"
	STATOM  = "ibc/C140AFD542AE77BD7DCC83F13FDD8C5E5BB8C4929785E6EC2F4C636F98F17901"
	USDC    = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	USDCaxl = "ibc/D189335C6E4A68B513C10AB227BF1C1D38C746766278BA3EEB4FB14124F1D858"
	USDT    = "ibc/4ABBEF4C8926DDDB320AE5188CFD63267ABBCEFC0583E4AE05D6E5AA2401DDAB"
	WBTC    = "ibc/D1542AA8762DB13087D8364F3EA6509FD6F009A34F00426AF9E4F9FA85CBBF1F"
	ETH     = "ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5"
	AKT     = "ibc/1480B8FD20AD5FCAE81EA87584D269547DD4D436843C1D20F15E00EB64743EF4"
	UMEE    = "ibc/67795E528DF67C5606FC20F824EA39A6EF55BA133F4DC79C90A8C47A0901E17C"
	UION    = "uion"
	CRE     = "ibc/5A7C219BA5F7582B99629BA3B2A01A61BFDA0F6FD1FE95B5366F7334C4BC0580"

	MainnetDenoms = []string{
		UOSMO,
		ATOM,
		STOSMO,
		STATOM,
		USDC,
		USDCaxl,
		USDT,
		WBTC,
		ETH,
		AKT,
		UMEE,
		UION,
		CRE,
	}

	// The files below are set in init()
	projectRoot              = ""
	absolutePathToStateFiles = ""

	DefaultRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:          []uint64{},
		MaxRoutes:                 4,
		MaxPoolsPerRoute:          4,
		MaxSplitRoutes:            4,
		MaxSplitIterations:        10,
		MinOSMOLiquidity:          20000,
		RouteUpdateHeightInterval: 0,
		RouteCacheEnabled:         true,
	}

	DefaultPoolsConfig = domain.PoolsConfig{
		// Transmuter V1 and V2
		TransmuterCodeIDs:      []uint64{148, 254},
		GeneralCosmWasmCodeIDs: []uint64{},
	}

	DefaultPricingRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:  []uint64{},
		MaxRoutes:         5,
		MaxPoolsPerRoute:  3,
		MaxSplitRoutes:    3,
		MinOSMOLiquidity:  50,
		RouteCacheEnabled: true,
	}

	DefaultPricingConfig = domain.PricingConfig{
		DefaultSource:          domain.ChainPricingSourceType,
		CacheExpiryMs:          2000,
		DefaultQuoteHumanDenom: "usdc",
		MaxPoolsPerRoute:       4,
		MaxRoutes:              5,
		MinOSMOLiquidity:       50,
	}
)

func init() {
	var err error
	projectRoot, err = findProjectRoot()
	if err != nil {
		panic(err)
	}

	absolutePathToStateFiles = projectRoot + relativePathMainnetFiles
}

// findProjectRoot starts from the current dir and goes up until it finds go.mod,
// returning the absolute directory containing it.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd() // Getwd already returns an absolute path
	if err != nil {
		return "", err
	}

	for {
		// Check for go.mod in the current directory
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// Ensure the path is absolute, even though it should already be
			return filepath.Abs(dir)
		}

		// Move up one directory level
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			// If the parent directory is the same as the current, we've reached the root
			break
		}
		dir = parentDir
	}

	return "", fmt.Errorf("project root not found")
}

func denomNum(i int) string {
	return fmt.Sprintf("denom%d", i)
}

// Note that it does not deep copy pools
func WithRoutePools(r route.RouteImpl, pools []sqsdomain.RoutablePool) route.RouteImpl {
	newRoute := route.RouteImpl{
		Pools: make([]sqsdomain.RoutablePool, 0, len(pools)),
	}

	newRoute.Pools = append(newRoute.Pools, pools...)

	return newRoute
}

// Note that it does not deep copy pools
func WithCandidateRoutePools(r sqsdomain.CandidateRoute, pools []sqsdomain.CandidatePool) sqsdomain.CandidateRoute {
	newRoute := sqsdomain.CandidateRoute{
		Pools: make([]sqsdomain.CandidatePool, 0, len(pools)),
	}

	newRoute.Pools = append(newRoute.Pools, pools...)
	return newRoute
}

// ValidateRoutePools validates that the expected pools are equal to the actual pools.
// Specifically, validates the following fields:
// - ID
// - Type
// - Balances
// - Spread Factor
// - Token Out Denom
// - Taker Fee
func (s *RouterTestHelper) ValidateRoutePools(expectedPools []sqsdomain.RoutablePool, actualPools []sqsdomain.RoutablePool) {
	s.Require().Equal(len(expectedPools), len(actualPools))

	for i, expectedPool := range expectedPools {
		actualPool := actualPools[i]

		expectedResultPool, ok := expectedPool.(domain.RoutableResultPool)
		s.Require().True(ok)

		// Cast to result pool
		actualResultPool, ok := actualPool.(domain.RoutableResultPool)
		s.Require().True(ok)

		s.Require().Equal(expectedResultPool.GetId(), actualResultPool.GetId())
		s.Require().Equal(expectedResultPool.GetType(), actualResultPool.GetType())
		s.Require().Equal(expectedResultPool.GetBalances().String(), actualResultPool.GetBalances().String())
		s.Require().Equal(expectedResultPool.GetSpreadFactor().String(), actualResultPool.GetSpreadFactor().String())
		s.Require().Equal(expectedResultPool.GetTokenOutDenom(), actualResultPool.GetTokenOutDenom())
		s.Require().Equal(expectedResultPool.GetTakerFee().String(), actualResultPool.GetTakerFee().String())
	}
}

func (s *RouterTestHelper) SetupDefaultMainnetRouter() (*routerusecase.Router, MockMainnetState) {
	routerConfig := DefaultRouterConfig
	pricingConfig := DefaultPricingConfig
	return s.SetupMainnetRouter(routerConfig, pricingConfig)
}

func (s *RouterTestHelper) SetupMainnetRouter(routerConfig domain.RouterConfig, pricingConfig domain.PricingConfig) (*routerusecase.Router, MockMainnetState) {
	pools, tickMap, err := parsing.ReadPools(absolutePathToStateFiles + poolsFileName)
	s.Require().NoError(err)

	takerFeeMap, err := parsing.ReadTakerFees(absolutePathToStateFiles + takerFeesFileName)
	s.Require().NoError(err)

	tokensMetadata, err := parsing.ReadTokensMetadata(absolutePathToStateFiles + tokensMetadataFileName)
	s.Require().NoError(err)

	// N.B. uncomment if logs are needed.
	// logger, err := log.NewLogger(false, "", "info")
	// s.Require().NoError(err)
	router := routerusecase.NewRouter(routerConfig, EmpyCosmWasmPoolRouterConfig, &log.NoOpLogger{})
	router = routerusecase.WithSortedPools(router, pools)

	return router, MockMainnetState{
		TickMap:        tickMap,
		TakerFeeMap:    takerFeeMap,
		TokensMetadata: tokensMetadata,
		PricingConfig:  pricingConfig,
	}
}

// Sets up and returns usecases for router and pools by mocking the mainnet data
// from json files.
func (s *RouterTestHelper) SetupRouterAndPoolsUsecase(router *routerusecase.Router, mainnetState MockMainnetState, cacheOpts ...CacheOption) MockMainnetUsecase {
	// Initialize empty caches
	cacheOptions := &CacheOptions{
		CandidateRoutes: cache.New(),
		RankedRoutes:    cache.New(),
		Pricing:         cache.New(),
	}

	// Apply cache options
	for _, opt := range cacheOpts {
		opt(cacheOptions)
	}

	// Setup router repository mock
	routerRepositoryMock := routerrepo.New()
	routerRepositoryMock.SetTakerFees(mainnetState.TakerFeeMap)
	routerusecase.WithComputedSortedPools(router, router.GetSortedPools())

	// Setup pools usecase mock.
	poolsUsecase := poolsusecase.NewPoolsUsecase(&DefaultPoolsConfig, "node-uri-placeholder", routerRepositoryMock)
	err := poolsUsecase.StorePools(router.GetSortedPools())
	s.Require().NoError(err)

	routerUsecase := routerusecase.NewRouterUsecase(routerRepositoryMock, poolsUsecase, router.GetConfig(), router.GetCosmWasmPoolConfig(), &log.NoOpLogger{}, cacheOptions.RankedRoutes, cacheOptions.CandidateRoutes)
	err = routerUsecase.SortPools(context.Background(), router.GetSortedPools())
	s.Require().NoError(err)

	tokensUsecase := tokensusecase.NewTokensUsecase(mainnetState.TokensMetadata)

	// Set up on-chain pricing strategy
	pricingSource, err := pricing.NewPricingStrategy(mainnetState.PricingConfig, tokensUsecase, routerUsecase)
	s.Require().NoError(err)

	pricingSource = pricing.WithPricingCache(pricingSource, cacheOptions.Pricing)

	tokensUsecase.RegisterPricingStrategy(domain.ChainPricingSourceType, pricingSource)

	encCfg := app.MakeEncodingConfig()

	ingestUsecase, err := ingestusecase.NewIngestUsecase(poolsUsecase, routerUsecase, nil, tokensUsecase, encCfg.Marshaler, mainnetState.PricingConfig, &log.NoOpLogger{})
	if err != nil {
		panic(err)
	}

	return MockMainnetUsecase{
		Pools:  poolsUsecase,
		Router: routerUsecase,
		Tokens: tokensUsecase,
		Ingest: ingestUsecase,
	}
}

// helper to convert any to BigDec
func (s *RouterTestHelper) ConvertAnyToBigDec(any any) osmomath.BigDec {
	bigDec, ok := any.(osmomath.BigDec)
	s.Require().True(ok)
	return bigDec
}
