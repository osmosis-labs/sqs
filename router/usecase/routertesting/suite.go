package routertesting

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	poolsusecase "github.com/osmosis-labs/sqs/pools/usecase"
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"
	"github.com/osmosis-labs/sqs/sqsdomain"
	sqsdomainmocks "github.com/osmosis-labs/sqs/sqsdomain/mocks"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v23/app/apptesting"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v23/x/poolmanager/types"
)

type RouterTestHelper struct {
	apptesting.ConcentratedKeeperTestHelper
}

// Mock mainnet state
type MockMainnetState struct {
	TickMap        map[uint64]sqsdomain.TickModel
	TakerFeeMap    sqsdomain.TakerFeeMap
	TokensMetadata map[string]domain.Token
}

type MockMainnetUsecase struct {
	Pools  mvc.PoolsUsecase
	Router mvc.RouterUsecase
	Tokens mvc.TokensUsecase
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
	EmptyRoute = route.RouteImpl{}

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
		RouteCacheEnabled:         false,
	}

	DefaultPoolsConfig = domain.PoolsConfig{
		// Transmuter V1 and V2
		TransmuterCodeIDs:      []uint64{148, 254},
		GeneralCosmWasmCodeIDs: []uint64{},
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
	routerConfig := domain.RouterConfig{
		PreferredPoolIDs:          []uint64{},
		MaxRoutes:                 4,
		MaxPoolsPerRoute:          4,
		MaxSplitIterations:        10,
		MinOSMOLiquidity:          10000,
		RouteUpdateHeightInterval: 0,
		RouteCacheEnabled:         false,
	}

	return s.SetupMainnetRouter(routerConfig)
}

func (s *RouterTestHelper) SetupMainnetRouter(config domain.RouterConfig) (*routerusecase.Router, MockMainnetState) {
	pools, tickMap, err := parsing.ReadPools(absolutePathToStateFiles + poolsFileName)
	s.Require().NoError(err)

	takerFeeMap, err := parsing.ReadTakerFees(absolutePathToStateFiles + takerFeesFileName)
	s.Require().NoError(err)

	tokensMetadata, err := parsing.ReadTokensMetadata(absolutePathToStateFiles + tokensMetadataFileName)
	s.Require().NoError(err)

	logger, err := log.NewLogger(false, "", "info")
	s.Require().NoError(err)
	router := routerusecase.NewRouter(config.PreferredPoolIDs, config.MaxPoolsPerRoute, config.MaxRoutes, config.MaxSplitRoutes, config.MaxSplitIterations, config.MinOSMOLiquidity, logger)
	router = routerusecase.WithSortedPools(router, pools)

	return router, MockMainnetState{
		TickMap:        tickMap,
		TakerFeeMap:    takerFeeMap,
		TokensMetadata: tokensMetadata,
	}
}

// Sets up and returns usecases for router and pools by mocking the mainnet data
// from json files.
func (s *RouterTestHelper) SetupRouterAndPoolsUsecase(router *routerusecase.Router, mainnetState MockMainnetState, rankedRoutesCache *cache.Cache, routesOverwrite *cache.RoutesOverwrite) MockMainnetUsecase {
	// Setup router repository mock
	routerRepositoryMock := sqsdomainmocks.RedisRouterRepositoryMock{}
	routerusecase.WithRouterRepository(router, &routerRepositoryMock)

	// Setup pools usecase mock.
	poolsRepositoryMock := sqsdomainmocks.RedisPoolsRepositoryMock{
		Pools:     router.GetSortedPools(),
		TickModel: mainnetState.TickMap,
	}
	poolsUsecase := poolsusecase.NewPoolsUsecase(time.Hour, &poolsRepositoryMock, nil, &DefaultPoolsConfig, "node-uri-placeholder")
	routerusecase.WithPoolsUsecase(router, poolsUsecase)

	routerUsecase := routerusecase.NewRouterUsecase(time.Hour, &routerRepositoryMock, poolsUsecase, DefaultRouterConfig, &log.NoOpLogger{}, rankedRoutesCache, routesOverwrite)

	tokensUsecase := tokensusecase.NewTokensUsecase(time.Hour, mainnetState.TokensMetadata)

	return MockMainnetUsecase{
		Pools:  poolsUsecase,
		Router: routerUsecase,
		Tokens: tokensUsecase,
	}
}
