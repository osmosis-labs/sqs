package routertesting

import (
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
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"
	"github.com/osmosis-labs/sqs/sqsdomain"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"

	routerrepo "github.com/osmosis-labs/sqs/router/repository"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v25/app"
	"github.com/osmosis-labs/osmosis/v25/app/apptesting"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
	coingeckopricing "github.com/osmosis-labs/sqs/tokens/usecase/pricing/coingecko"
)

type RouterTestHelper struct {
	apptesting.ConcentratedKeeperTestHelper
}

// Mock mainnet state
type MockMainnetState struct {
	Pools                    []sqsdomain.PoolI
	TickMap                  map[uint64]*sqsdomain.TickModel
	TakerFeeMap              sqsdomain.TakerFeeMap
	TokensMetadata           map[string]domain.Token
	PricingConfig            domain.PricingConfig
	CandidateRouteSearchData map[string]domain.CandidateRouteDenomData
	PoolDenomsMetaData       domain.PoolDenomMetaDataMap
}

type MockMainnetUsecase struct {
	Pools                  mvc.PoolsUsecase
	Router                 mvc.RouterUsecase
	Tokens                 mvc.TokensUsecase
	Ingest                 mvc.IngestUsecase
	CandidateRouteSearcher domain.CandidateRouteSearcher
}

const (
	DefaultPoolID = uint64(1)

	relativePathMainnetFiles   = "/router/usecase/routertesting/parsing/"
	poolsFileName              = "pools.json"
	takerFeesFileName          = "taker_fees.json"
	tokensMetadataFileName     = "tokens.json"
	candidateRouteFileName     = "candidate_route_search_data.json"
	poolDenomsMetaDataFileName = "pool_denom_metadata.json"
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
		ID:               DefaultPoolID,
		Denoms:           []string{DenomOne, DenomTwo},
		PoolLiquidityCap: osmomath.NewInt(10),
		PoolType:         poolmanagertypes.Balancer,
		Balances:         DefaultPoolBalances,
		TakerFee:         DefaultTakerFee,
		SpreadFactor:     DefaultSpreadFactor,
	}
	EmptyRoute                   = route.RouteImpl{}
	EmpyCosmWasmPoolRouterConfig = domain.CosmWasmPoolRouterConfig{
		TransmuterCodeIDs:        map[uint64]struct{}{},
		GeneralCosmWasmCodeIDs:   map[uint64]struct{}{},
		ChainGRPCGatewayEndpoint: "",
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
	TIA     = "ibc/D79E7D83AB399BFFF93433E54FAA480C191248FC556924A2A8351AE2638B3877"
	UION    = "uion"
	CRE     = "ibc/5A7C219BA5F7582B99629BA3B2A01A61BFDA0F6FD1FE95B5366F7334C4BC0580"
	STEVMOS = "ibc/C5579A9595790017C600DD726276D978B9BF314CF82406CE342720A9C7911A01"
	// DYDX is 18 decimals
	DYDX        = "ibc/831F0B1BBB1D08A2B75311892876D71565478C532967545476DF4C2D7492E48C"
	ALLUSDT     = "factory/osmo1em6xs47hd82806f5cxgyufguxrrc7l0aqx7nzzptjuqgswczk8csavdxek/alloyed/allUSDT"
	ALLBTC      = "factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC"
	KAVAUSDT    = "ibc/4ABBEF4C8926DDDB320AE5188CFD63267ABBCEFC0583E4AE05D6E5AA2401DDAB"
	EVMOS       = "ibc/6AE98883D4D5D5FF9E50D7130F1305DA2FFA0C652D1DD9C123657C6B4EB2DF8A"
	NATIVE_WBTC = "factory/osmo1z0qrq605sjgcqpylfl4aa6s90x738j7m58wyatt0tdzflg2ha26q67k743/wbtc"

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
		PreferredPoolIDs:    []uint64{},
		MaxRoutes:           20,
		MaxPoolsPerRoute:    4,
		MaxSplitRoutes:      3,
		MinPoolLiquidityCap: 1000,
		RouteCacheEnabled:   true,

		// Set proper dynamic min liquidity config here
		DynamicMinLiquidityCapFiltersDesc: []domain.DynamicMinLiquidityCapFilterEntry{
			{
				// 1_000_000 min token liquidity capitalization translates to a 75_000 filter value
				MinTokensCap: 100000,
				FilterValue:  75000,
			},
			{
				MinTokensCap: 250000,
				FilterValue:  15000,
			},
			{
				MinTokensCap: 10000,
				FilterValue:  1000,
			},
			{
				MinTokensCap: 1000,
				FilterValue:  10,
			},
			{
				MinTokensCap: 1,
				FilterValue:  1,
			},
		},
	}

	DefaultPoolsConfig = domain.PoolsConfig{
		// Transmuter V1 and V2
		TransmuterCodeIDs:        []uint64{148, 254},
		AlloyedTransmuterCodeIDs: []uint64{814},
		OrderbookCodeIDs:         []uint64{885},
		GeneralCosmWasmCodeIDs:   []uint64{},
	}

	DefaultPricingRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:    []uint64{},
		MaxRoutes:           5,
		MaxPoolsPerRoute:    3,
		MaxSplitRoutes:      3,
		MinPoolLiquidityCap: 50,
		RouteCacheEnabled:   true,
	}

	DefaultPricingConfig = domain.PricingConfig{
		DefaultSource:             domain.ChainPricingSourceType,
		CacheExpiryMs:             2000,
		DefaultQuoteHumanDenom:    "usdc",
		MaxPoolsPerRoute:          4,
		MaxRoutes:                 5,
		MinPoolLiquidityCap:       50,
		CoingeckoUrl:              "https://prices.osmosis.zone/api/v3/simple/price",
		CoingeckoQuoteCurrency:    "usd",
		WorkerMinPoolLiquidityCap: 5,
	}

	emptyCosmwasmPoolRouterConfig = domain.CosmWasmPoolRouterConfig{}

	// UnsetScalingFactorGetterCb is a callback that is unset by default for various tests
	// due to no need.
	UnsetScalingFactorGetterCb domain.ScalingFactorGetterCb = func(denom string) (osmomath.Dec, error) {
		// Note: for many tests the scaling factor getter cb is irrelevant.
		// As a result, we unset it for simplicity.
		// If you run into this panic, your test might benefit from properly wiring the scaling factor
		// getter callback (defined on the tokens use case)
		panic("scaling factor getter cb is unset")
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
func WithRoutePools(r route.RouteImpl, pools []domain.RoutablePool) route.RouteImpl {
	newRoute := route.RouteImpl{
		HasCanonicalOrderbookPool: r.HasCanonicalOrderbookPool,
		Pools:                     make([]domain.RoutablePool, 0, len(pools)),
	}

	newRoute.Pools = append(newRoute.Pools, pools...)

	return newRoute
}

// Note that it does not deep copy pools
func WithCandidateRoutePools(r sqsdomain.CandidateRoute, pools []sqsdomain.CandidatePool) sqsdomain.CandidateRoute {
	newRoute := sqsdomain.CandidateRoute{
		IsCanonicalOrderboolRoute: r.IsCanonicalOrderboolRoute,
		Pools:                     make([]sqsdomain.CandidatePool, 0, len(pools)),
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
func (s *RouterTestHelper) ValidateRoutePools(expectedPools []domain.RoutablePool, actualPools []domain.RoutablePool) {
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

func (s *RouterTestHelper) SetupMainnetState() MockMainnetState {
	pools, tickMap, err := parsing.ReadPools(absolutePathToStateFiles + poolsFileName)
	s.Require().NoError(err)

	takerFeeMap, err := parsing.ReadTakerFees(absolutePathToStateFiles + takerFeesFileName)
	s.Require().NoError(err)

	tokensMetadata, err := parsing.ReadTokensMetadata(absolutePathToStateFiles + tokensMetadataFileName)
	s.Require().NoError(err)

	poolDenomsMetaData, err := parsing.ReadPoolDenomsMetaData(absolutePathToStateFiles + poolDenomsMetaDataFileName)
	s.Require().NoError(err)

	candidateRouteSearchData, err := parsing.ReadCandidateRouteSearchData(absolutePathToStateFiles + candidateRouteFileName)
	s.Require().NoError(err)

	return MockMainnetState{
		Pools:                    pools,
		TickMap:                  tickMap,
		TakerFeeMap:              takerFeeMap,
		TokensMetadata:           tokensMetadata,
		CandidateRouteSearchData: candidateRouteSearchData,
		PoolDenomsMetaData:       poolDenomsMetaData,
	}
}

// Sets up and returns usecases for router and pools by mocking the mainnet data
// from json files.
func (s *RouterTestHelper) SetupRouterAndPoolsUsecase(mainnetState MockMainnetState, cacheOpts ...MainnetTestOption) MockMainnetUsecase {
	// Initialize empty caches
	options := &MainnetTestOptions{
		CandidateRoutes:  cache.New(),
		RankedRoutes:     cache.New(),
		Pricing:          cache.New(),
		RouterConfig:     DefaultRouterConfig,
		PricingConfig:    DefaultPricingConfig,
		PoolsConfig:      DefaultPoolsConfig,
		IsLoggerDisabled: false,
	}

	// Apply cache options
	for _, opt := range cacheOpts {
		opt(options)
	}

	var (
		logger log.Logger = &log.NoOpLogger{}
		err    error
	)
	if !options.IsLoggerDisabled {
		logger, err = log.NewLogger(false, "", "info")
		s.Require().NoError(err)
	}

	// Setup router repository mock
	routerRepositoryMock := routerrepo.New(&log.NoOpLogger{})
	routerRepositoryMock.SetTakerFees(mainnetState.TakerFeeMap)
	routerRepositoryMock.SetCandidateRouteSearchData(mainnetState.CandidateRouteSearchData)

	// Setup pools usecase mock.
	poolsUsecase, err := poolsusecase.NewPoolsUsecase(&options.PoolsConfig, "node-uri-placeholder", routerRepositoryMock, domain.UnsetScalingFactorGetterCb, &log.NoOpLogger{})
	s.Require().NoError(err)
	err = poolsUsecase.StorePools(mainnetState.Pools)
	s.Require().NoError(err)

	tokensUsecase := tokensusecase.NewTokensUsecase(mainnetState.TokensMetadata, 0, &log.NoOpLogger{})
	tokensUsecase.UpdatePoolDenomMetadata(mainnetState.PoolDenomsMetaData)

	candidateRouteFinder := routerusecase.NewCandidateRouteFinder(routerRepositoryMock, logger)

	routerUsecase := routerusecase.NewRouterUsecase(routerRepositoryMock, poolsUsecase, candidateRouteFinder, tokensUsecase, options.RouterConfig, poolsUsecase.GetCosmWasmPoolConfig(), logger, options.RankedRoutes, options.CandidateRoutes)

	pricingRouterUsecase := routerusecase.NewRouterUsecase(routerRepositoryMock, poolsUsecase, candidateRouteFinder, tokensUsecase, options.RouterConfig, poolsUsecase.GetCosmWasmPoolConfig(), logger, cache.New(), cache.New())

	// Validate and sort pools
	sortedPools, _ := routerusecase.ValidateAndSortPools(mainnetState.Pools, poolsUsecase.GetCosmWasmPoolConfig(), options.RouterConfig.PreferredPoolIDs, logger)

	routerUsecase.SetSortedPools(sortedPools)

	// Set up on-chain pricing strategy
	pricingSource, err := pricing.NewPricingStrategy(options.PricingConfig, tokensUsecase, routerUsecase)
	s.Require().NoError(err)

	pricingSource = pricing.WithPricingCache(pricingSource, options.Pricing)

	tokensUsecase.RegisterPricingStrategy(domain.ChainPricingSourceType, pricingSource)

	// Set up Coingecko pricing strategy, use MockCoingeckoPriceGetter for testing purposes
	options.PricingConfig.DefaultSource = domain.CoinGeckoPricingSourceType
	coingeckoPricingSource := coingeckopricing.New(tokensUsecase, options.PricingConfig, mocks.DefaultMockCoingeckoPriceGetter)
	s.Require().NoError(err)
	tokensUsecase.RegisterPricingStrategy(domain.CoinGeckoPricingSourceType, coingeckoPricingSource)

	encCfg := app.MakeEncodingConfig()

	ingestUsecase, err := ingestusecase.NewIngestUsecase(poolsUsecase, routerUsecase, pricingRouterUsecase, tokensUsecase, nil, encCfg.Marshaler, nil, nil, nil, logger)
	if err != nil {
		panic(err)
	}

	return MockMainnetUsecase{
		Pools:                  poolsUsecase,
		Router:                 routerUsecase,
		Tokens:                 tokensUsecase,
		Ingest:                 ingestUsecase,
		CandidateRouteSearcher: candidateRouteFinder,
	}
}

// helper to convert any to BigDec
func (s *RouterTestHelper) ConvertAnyToBigDec(any any) osmomath.BigDec {
	bigDec, ok := any.(osmomath.BigDec)
	s.Require().True(ok)
	return bigDec
}

// PrepareValidSortedRouterPools prepares a list of valid router pools above min liquidity
func PrepareValidSortedRouterPools(pools []sqsdomain.PoolI, minPoolLiquidityCap uint64) []sqsdomain.PoolI {
	sortedPools, _ := routerusecase.ValidateAndSortPools(pools, emptyCosmwasmPoolRouterConfig, []uint64{}, &log.NoOpLogger{})

	// Sort pools
	poolsAboveMinLiquidity := routerusecase.FilterPoolsByMinLiquidity(sortedPools, minPoolLiquidityCap)

	return poolsAboveMinLiquidity
}
