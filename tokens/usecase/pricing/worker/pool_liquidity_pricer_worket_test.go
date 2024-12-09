package worker_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
	"github.com/stretchr/testify/suite"
)

type PoolLiquidityComputeWorkerSuite struct {
	routertesting.RouterTestHelper
}

type liquidityResult struct {
	LiquidityCap      osmomath.Int
	LiquidityCapError string
}

const (
	pricingCacheExpiry = 2000

	defaultUpdateHeight uint64 = 2

	defaultPoolID uint64 = 1
)

var (
	defaultPricingRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:    []uint64{},
		MaxRoutes:           5,
		MaxPoolsPerRoute:    3,
		MaxSplitRoutes:      3,
		MinPoolLiquidityCap: 50,
		RouteCacheEnabled:   true,
	}

	defaultScalingFactor = osmomath.NewDec(1_000_000)

	zeroCapitalization = osmomath.ZeroInt()

	zeroPrice = osmomath.ZeroBigDec()

	defaultPrice     = osmomath.NewBigDec(2)
	defaultLiquidity = osmomath.NewInt(1_000_000)

	defaultLiquidityCap = defaultLiquidity.ToLegacyDec().Quo(defaultScalingFactor).MulMut(defaultPrice.Dec()).TruncateInt()

	// Note: we are not testing the error handling of underlying methods.
	// Those are unit-tested in their respective tests.
	// As a result, we only set up the valid cases here.
	defaultScalingFactorMap = map[string]osmomath.Dec{
		UOSMO: defaultScalingFactor,
		USDC:  defaultScalingFactor,
		ATOM:  defaultScalingFactor,
	}

	defaultBlockPriceUpdates = domain.PricesResult{
		UOSMO: {
			USDC: defaultPrice,
		},
		ATOM: {
			USDC: defaultPrice,
		},
	}

	defaultBlockLiquidityUpdates = domain.DenomPoolLiquidityMap{
		UOSMO: {
			TotalLiquidity: defaultLiquidity,
		},
	}

	defaultBlockPoolMetaData = domain.BlockPoolMetadata{
		UpdatedDenoms: map[string]struct{}{
			UOSMO: {},
		},
		DenomPoolLiquidityMap: defaultBlockLiquidityUpdates,

		PoolIDs: map[uint64]struct{}{
			defaultPoolID: {},
		},
	}

	defaultUOSMOBalance = sdk.NewCoin(UOSMO, defaultLiquidity)

	defaultATOMBalance = sdk.NewCoin(ATOM, defaultLiquidity)
)

var (
	stableCoinDenoms = []string{"usdc", "usdt", "dai", "ist"}
)

func TestPoolLiquidityComputeWorkerSuite(t *testing.T) {
	suite.Run(t, new(PoolLiquidityComputeWorkerSuite))
}

// This is a test validating that given valid price updates
// and block liquidity data, the worker compuetes the pool liqudity capitalization
// overwriting the data in the poolLiquidityHandlerMock.
//
// This is a functional test that tests given correct updates,
// they are propagated in the pool liquidity handler as intended.
// The edge cases of each underlying component are tested by their corresponding unit tests.
func (s *PoolLiquidityComputeWorkerSuite) TestOnPricingUpdate() {
	// Create liquidity pricer
	liquidityPricer := worker.NewLiquidityPricer(USDC, mocks.SetupMockScalingFactorCbFromMap(defaultScalingFactorMap))

	// Set up the tokens pool liquidity mock handler
	poolLiquidityHandlerMock := mocks.TokensPoolLiquidityHandlerMock{
		DenomScalingFactorMap: defaultScalingFactorMap,

		// Note we pre-set zero liquidity cap with zero price
		// to ensure that these are overwritten.
		PoolDenomMetadataMap: domain.PoolDenomMetaDataMap{
			UOSMO: domain.PoolDenomMetaData{
				Price:             zeroPrice,
				TotalLiquidity:    defaultLiquidity,
				TotalLiquidityCap: zeroCapitalization,
			},
		},
	}

	poolHandlerMock := mocks.PoolHandlerMock{
		Pools: []sqsdomain.PoolI{&mocks.MockRoutablePool{ID: defaultPoolID, Balances: sdk.NewCoins(defaultUOSMOBalance)}},
	}

	// Create the worker
	poolLiquidityPricerWorker := worker.NewPoolLiquidityWorker(&poolLiquidityHandlerMock, &poolHandlerMock, liquidityPricer, &log.NoOpLogger{})

	// Create & register mock listener
	mockListener := &mocks.PoolLiquidityPricingMock{}
	poolLiquidityPricerWorker.RegisterListener(mockListener)

	// System under test
	err := poolLiquidityPricerWorker.OnPricingUpdate(context.TODO(), defaultHeight, domain.BlockPoolMetadata{
		UpdatedDenoms: map[string]struct{}{
			UOSMO: {},
		},
		DenomPoolLiquidityMap: defaultBlockLiquidityUpdates,
		PoolIDs: map[uint64]struct{}{
			defaultPoolID: {},
		},
	}, defaultBlockPriceUpdates, USDC)

	s.Require().NoError(err)

	// Validate one pool denom metadata entry is present for UOSMO.
	s.Require().Len(poolLiquidityHandlerMock.PoolDenomMetadataMap, 1)
	result, ok := poolLiquidityHandlerMock.PoolDenomMetadataMap[UOSMO]
	s.Require().True(ok)

	// Assert on specific values.
	s.Require().Equal(result.Price, defaultPrice)
	s.Require().Equal(result.TotalLiquidity, defaultLiquidity)
	s.Require().Equal(result.TotalLiquidityCap.String(), defaultLiquidityCap.String())

	// Validate that the listener mock was called with the relevant height.
	lastHeightCalled := mockListener.GetLastHeightCalled()
	s.Require().Equal(defaultHeight, lastHeightCalled)

	// Validate that the pool liquidity handler mock was called with the relevant pool IDs.
	s.validateLiquidityCapPools(map[uint64]liquidityResult{
		defaultPoolID: {
			LiquidityCap: defaultLiquidityCap,
		},
	}, poolHandlerMock.Pools)
}

// TestHasLaterUpdateThanHeight tests the HasLaterUpdateThanHeight method by following the spec.
func (s *PoolLiquidityComputeWorkerSuite) TestHasLaterUpdateThanHeight() {
	const defaultHeight = 1

	var (
		defaultDenom = UOSMO
		otherDenom   = USDC
	)

	tests := []struct {
		name string

		preSetHeightForDenom map[string]uint64

		denom  string
		height uint64

		expected bool
	}{
		{
			name: "no pre-set",

			denom:  defaultDenom,
			height: defaultHeight,

			expected: false,
		},
		{
			name: "pre-set smaller than height",

			preSetHeightForDenom: map[string]uint64{
				defaultDenom: defaultHeight - 1,
			},

			denom:  defaultDenom,
			height: defaultHeight,

			expected: false,
		},
		{
			name: "pre-set equal height",

			preSetHeightForDenom: map[string]uint64{
				defaultDenom: defaultHeight,
			},

			denom:  defaultDenom,
			height: defaultHeight,

			expected: false,
		},
		{
			name: "pre-set greater height",

			preSetHeightForDenom: map[string]uint64{
				defaultDenom: defaultHeight + 1,
			},

			denom:  defaultDenom,
			height: defaultHeight,

			expected: true,
		},

		{
			name: "pre-set multi-denom, other denom greater height",

			preSetHeightForDenom: map[string]uint64{
				defaultDenom: defaultHeight,
				otherDenom:   defaultHeight + 1,
			},

			denom:  defaultDenom,
			height: defaultHeight,

			expected: false,
		},

		{
			name: "pre-set multi-denom, input denom greater height",

			preSetHeightForDenom: map[string]uint64{
				defaultDenom: defaultHeight,
				otherDenom:   defaultHeight + 1,
			},

			denom:  otherDenom,
			height: defaultHeight,

			expected: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

			poolLiquidityPricerWorker := &worker.PoolLiquidityPricerWorker{}

			// Initialize the height for each denom.
			for denom, height := range tt.preSetHeightForDenom {
				poolLiquidityPricerWorker.StoreHeightForDenom(denom, height)
			}

			// System under test.
			actual := poolLiquidityPricerWorker.HasLaterUpdateThanHeight(tt.denom, tt.height)

			// Check the result.
			s.Require().Equal(tt.expected, actual)
		})
	}
}

// TestRepriceDenomsMetadata tests the StoreHeightForDenom method by following the spec.
func (s *PoolLiquidityComputeWorkerSuite) TestRepriceDenomsMetadata() {
	var (
		defaultUOSMOHeightResult = map[string]uint64{
			UOSMO: defaultUpdateHeight,
		}

		zeroUOSMOHeightResult = map[string]uint64{
			UOSMO: 0,
		}

		laterUOSMOHeightResult = map[string]uint64{
			UOSMO: defaultUpdateHeight + 1,
		}

		// When we fail to retrieve price and, as a result, compute the liquidity capitalization,
		// this is what is returned instead of failing.
		defaultZeroPricePoolDenomMetaDataMapResult = domain.PoolDenomMetaDataMap{
			UOSMO: {
				// Note: set to zero
				Price:          zeroPrice,
				TotalLiquidity: defaultLiquidity,
				// Note: set to zero
				TotalLiquidityCap: zeroCapitalization,
			},
		}
	)

	tests := []struct {
		name string

		preSetUpdateHeightForDenom map[string]uint64

		updateHeight      uint64
		blockPriceUpdates domain.PricesResult
		quoteDenom        string
		blockPoolMetaData domain.BlockPoolMetadata

		expectedUpdatedDenomMetadata domain.PoolDenomMetaDataMap

		expectedDenomHeights map[string]uint64
	}{
		{
			name: "one denom success case",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			blockPoolMetaData: defaultBlockPoolMetaData,

			expectedUpdatedDenomMetadata: domain.PoolDenomMetaDataMap{
				UOSMO: {
					Price:             defaultPrice,
					TotalLiquidity:    defaultLiquidity,
					TotalLiquidityCap: defaultLiquidityCap,
				},
			},

			expectedDenomHeights: defaultUOSMOHeightResult,
		},
		{
			name: "zero denoms -> empty result",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			blockPoolMetaData: domain.BlockPoolMetadata{
				UpdatedDenoms:         map[string]struct{}{},
				DenomPoolLiquidityMap: domain.DenomPoolLiquidityMap{},
			},
			expectedUpdatedDenomMetadata: domain.PoolDenomMetaDataMap{},

			expectedDenomHeights: zeroUOSMOHeightResult,
		},

		{
			name: "one denom - later update exists the height and metadata are not updated",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			// Note: pre-set to this.
			preSetUpdateHeightForDenom: laterUOSMOHeightResult,

			blockPoolMetaData: defaultBlockPoolMetaData,

			// Updates are not applied.
			expectedUpdatedDenomMetadata: domain.PoolDenomMetaDataMap{},

			// Height is not updated.
			expectedDenomHeights: laterUOSMOHeightResult,
		},

		{
			name: "one token, the price is not present, setting the pool liquidity capitalization to zero.",

			updateHeight: defaultUpdateHeight,
			// Note: no price for UOSMO
			blockPriceUpdates: domain.PricesResult{},
			quoteDenom:        USDC,

			blockPoolMetaData: defaultBlockPoolMetaData,

			// Note: zero price result.
			expectedUpdatedDenomMetadata: defaultZeroPricePoolDenomMetaDataMapResult,

			expectedDenomHeights: defaultUOSMOHeightResult,
		},

		{
			name: "one denom, the quote denom is for a different price, setting the pool liquidity capitalization to zero",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			// Note: different quote denom
			quoteDenom: ATOM,

			// Note: zero price result.
			blockPoolMetaData: defaultBlockPoolMetaData,

			expectedUpdatedDenomMetadata: defaultZeroPricePoolDenomMetaDataMapResult,

			expectedDenomHeights: defaultUOSMOHeightResult,
		},

		{
			name: "two denoms correctly updated",

			updateHeight: defaultUpdateHeight,
			blockPriceUpdates: domain.PricesResult{
				UOSMO: {
					USDC: defaultPrice,
				},
				ATOM: {
					// Note 0.5 default price
					USDC: defaultPrice.QuoRaw(2),
				},
			},
			quoteDenom: USDC,

			blockPoolMetaData: domain.BlockPoolMetadata{
				UpdatedDenoms: map[string]struct{}{
					UOSMO: {},
					ATOM:  {},
				},
				DenomPoolLiquidityMap: domain.DenomPoolLiquidityMap{
					UOSMO: {
						TotalLiquidity: defaultLiquidity,
					},
					ATOM: {
						// 2x the liquidity
						TotalLiquidity: defaultLiquidity.Add(defaultLiquidity),
					},
				},
			},

			expectedUpdatedDenomMetadata: domain.PoolDenomMetaDataMap{
				UOSMO: {
					Price:             defaultPrice,
					TotalLiquidity:    defaultLiquidity,
					TotalLiquidityCap: defaultLiquidityCap,
				},
				ATOM: {
					Price:          defaultPrice.QuoRaw(2),
					TotalLiquidity: defaultLiquidity.Add(defaultLiquidity),
					// 0.5 price * 2 default liquidity yields the same capitalization
					// result as UOSMO.
					TotalLiquidityCap: defaultLiquidityCap,
				},
			},

			expectedDenomHeights: map[string]uint64{
				UOSMO: defaultUpdateHeight,
				ATOM:  defaultUpdateHeight,
			},
		},
		{
			name: "one token, denom liquidity map is no present -> liquidity cap is zero",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			blockPoolMetaData: domain.BlockPoolMetadata{
				UpdatedDenoms: map[string]struct{}{
					UOSMO: {},
				},
				// Note: no denom liquidity map set
			},

			// Note: empty result
			expectedUpdatedDenomMetadata: domain.PoolDenomMetaDataMap{
				UOSMO: {
					Price:             defaultPrice,
					TotalLiquidity:    osmomath.ZeroInt(),
					TotalLiquidityCap: zeroCapitalization,
				},
			},

			expectedDenomHeights: defaultUOSMOHeightResult,
		},
		{
			name: "one token, updated denom is not set -> skipped",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			blockPoolMetaData: domain.BlockPoolMetadata{
				// Note: no updated denoms set
				DenomPoolLiquidityMap: defaultBlockLiquidityUpdates,
			},

			// Note: empty result
			expectedUpdatedDenomMetadata: domain.PoolDenomMetaDataMap{},

			expectedDenomHeights: zeroUOSMOHeightResult,
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

			// Create liquidity pricer
			liquidityPricer := worker.NewLiquidityPricer(USDC, mocks.SetupMockScalingFactorCbFromMap(defaultScalingFactorMap))

			// Set up the tokens pool liquidity mock handler
			poolLiquidityHandlerMock := mocks.TokensPoolLiquidityHandlerMock{
				DenomScalingFactorMap: defaultScalingFactorMap,
			}

			// Create the worker
			poolLiquidityPricerWorker := worker.NewPoolLiquidityWorker(&poolLiquidityHandlerMock, nil, liquidityPricer, &log.NoOpLogger{})

			// Pre-set the height for each denom.
			for denom, height := range tt.preSetUpdateHeightForDenom {
				poolLiquidityPricerWorker.StoreHeightForDenom(denom, height)
			}

			// System under test
			poolDenomMetadata := poolLiquidityPricerWorker.RepriceDenomsMetadata(tt.updateHeight, tt.blockPriceUpdates, tt.quoteDenom, tt.blockPoolMetaData)

			// Check the result
			s.validatePoolDenomMetadata(tt.expectedUpdatedDenomMetadata, poolDenomMetadata)

			// Check the height is stored correctly
			for denom, expectedHeight := range tt.expectedDenomHeights {
				actualHeight := poolLiquidityPricerWorker.GetHeightForDenom(denom)

				s.Require().Equal(expectedHeight, actualHeight)
			}
		})
	}
}

func (s *PoolLiquidityComputeWorkerSuite) TestCreatePoolDenomMetaData() {
	tests := []struct {
		name string

		updatedBlockDenom  string
		preSetUpdateHeight uint64
		updateHeight       uint64
		blockPriceUpdates  domain.PricesResult
		quoteDenom         string
		blockPoolMetadata  domain.BlockPoolMetadata

		expectedPoolDenomMetadData domain.PoolDenomMetaData
		expectedErr                error
	}{
		{
			name: "happy path",

			updatedBlockDenom: UOSMO,
			updateHeight:      defaultHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,
			blockPoolMetadata: defaultBlockPoolMetaData,

			expectedPoolDenomMetadData: domain.PoolDenomMetaData{
				Price:             defaultPrice,
				TotalLiquidity:    defaultLiquidity,
				TotalLiquidityCap: defaultLiquidityCap,
			},
		},
		{
			name: "error: denom pool liquidity data not found",

			updatedBlockDenom: UOSMO,
			updateHeight:      defaultHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,
			blockPoolMetadata: domain.BlockPoolMetadata{
				// Empty map
				DenomPoolLiquidityMap: domain.DenomPoolLiquidityMap{},
			},

			expectedErr: domain.DenomPoolLiquidityDataNotFoundError{
				Denom: UOSMO,
			},
		},
		{
			name: "error: no price for denom",

			updatedBlockDenom: UOSMO,
			updateHeight:      defaultHeight,
			blockPriceUpdates: domain.PricesResult{},
			quoteDenom:        USDC,
			blockPoolMetadata: defaultBlockPoolMetaData,

			expectedErr: domain.PriceNotFoundForPoolLiquidityCapError{
				Denom: UOSMO,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

			// Create liquidity pricer
			liquidityPricer := worker.NewLiquidityPricer(USDC, mocks.SetupMockScalingFactorCbFromMap(defaultScalingFactorMap))

			// Set up the tokens pool liquidity mock handler
			poolLiquidityHandlerMock := mocks.TokensPoolLiquidityHandlerMock{
				DenomScalingFactorMap: defaultScalingFactorMap,
			}

			// Create the worker
			poolLiquidityPricerWorker := worker.NewPoolLiquidityWorker(&poolLiquidityHandlerMock, nil, liquidityPricer, &log.NoOpLogger{})

			// Pre-set the height for the denom.
			poolLiquidityPricerWorker.StoreHeightForDenom(tt.updatedBlockDenom, tt.preSetUpdateHeight)

			// System under test
			poolDenomMetadata, err := poolLiquidityPricerWorker.CreatePoolDenomMetaData(tt.updatedBlockDenom, tt.updateHeight, tt.blockPriceUpdates, tt.quoteDenom, tt.blockPoolMetadata)

			if tt.expectedErr != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(tt.expectedErr, err)
				return
			}

			s.Require().NoError(err)
			s.Require().Equal(tt.expectedPoolDenomMetadData, poolDenomMetadata)
		})

	}
}

// Tests the helper for determining if denom repricing should be skipped.
func (s *PoolLiquidityComputeWorkerSuite) TestShouldSkipDenomRepricing() {
	tests := []struct {
		name string

		updatedBlockDenom  string
		preSetUpdateHeight uint64
		updateHeight       uint64

		expected bool
	}{
		{
			name: "do not skip",

			updatedBlockDenom: UOSMO,
			updateHeight:      defaultHeight,

			expected: false,
		},
		{
			name: "skip: later update height present",

			updatedBlockDenom: UOSMO,
			// pre-set later update height.
			preSetUpdateHeight: defaultHeight + 1,
			updateHeight:       defaultHeight,

			expected: true,
		},
		{
			name: "skip: denom is gamm share",

			updatedBlockDenom: worker.GammSharePrefix,
			// pre-set later update height.
			preSetUpdateHeight: defaultHeight + 1,
			updateHeight:       defaultHeight,

			expected: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {
			// Create the worker
			// Note: all inputs are irrelevant for this test.
			poolLiquidityPricerWorker := worker.NewPoolLiquidityWorker(nil, nil, nil, &log.NoOpLogger{})

			// Pre-set the height for the denom.
			poolLiquidityPricerWorker.StoreHeightForDenom(tt.updatedBlockDenom, tt.preSetUpdateHeight)

			// System under test
			actual := poolLiquidityPricerWorker.ShouldSkipDenomRepricing(tt.updatedBlockDenom, tt.updateHeight)

			s.Require().Equal(tt.expected, actual)
		})

	}
}

func (s *PoolLiquidityComputeWorkerSuite) TestRepricePoolLiquidityCap() {
	tests := []struct {
		name string

		existingPools []sqsdomain.PoolI
		poolIDs       map[uint64]struct{}

		// updateHeight      uint64
		blockPriceUpdates domain.PricesResult
		quoteDenom        string

		expectedLiquidityResultByID map[uint64]liquidityResult

		expectError error
	}{
		{
			name: "one pool, one coin in balance",

			poolIDs: map[uint64]struct{}{
				defaultPoolID: {},
			},

			existingPools:     []sqsdomain.PoolI{&mocks.MockRoutablePool{ID: defaultPoolID, Balances: sdk.NewCoins(defaultUOSMOBalance)}},
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			expectedLiquidityResultByID: map[uint64]liquidityResult{
				defaultPoolID: {
					LiquidityCap: defaultLiquidityCap,
				},
			},
		},
		{
			name: "two pools, one with two coins in balance",

			poolIDs: map[uint64]struct{}{
				defaultPoolID:     {},
				defaultPoolID + 1: {},
			},

			existingPools: []sqsdomain.PoolI{
				// UOSMO: 1x default balance, ATOM: 1x default balance
				&mocks.MockRoutablePool{ID: defaultPoolID, Balances: sdk.NewCoins(defaultUOSMOBalance, defaultATOMBalance)},

				// UOSMO: 3x default balance
				&mocks.MockRoutablePool{ID: defaultPoolID + 1, Balances: sdk.NewCoins(defaultUOSMOBalance.Add(defaultUOSMOBalance).Add(defaultUOSMOBalance))},
			},
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			expectedLiquidityResultByID: map[uint64]liquidityResult{
				defaultPoolID: {
					LiquidityCap: defaultLiquidityCap.Add(defaultLiquidityCap),
				},

				defaultPoolID + 1: {
					LiquidityCap: defaultLiquidityCap.Add(defaultLiquidityCap).Add(defaultLiquidityCap),
				},
			},
		},
		{
			name: "two pools in state but only one is refreshed, one with two coins in balance",

			poolIDs: map[uint64]struct{}{
				// Only default + 1 is refreshed
				defaultPoolID + 1: {},
			},

			existingPools: []sqsdomain.PoolI{
				// UOSMO: 1x default balance, ATOM: 1x default balance, zero capitalization set
				&mocks.MockRoutablePool{ID: defaultPoolID, Balances: sdk.NewCoins(defaultUOSMOBalance, defaultATOMBalance), PoolLiquidityCap: zeroCapitalization},

				// UOSMO: 3x default balance
				&mocks.MockRoutablePool{ID: defaultPoolID + 1, Balances: sdk.NewCoins(defaultUOSMOBalance.Add(defaultUOSMOBalance).Add(defaultUOSMOBalance))},
			},
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			expectedLiquidityResultByID: map[uint64]liquidityResult{
				defaultPoolID: {
					LiquidityCap: zeroCapitalization,
				},

				defaultPoolID + 1: {
					LiquidityCap: defaultLiquidityCap.Add(defaultLiquidityCap).Add(defaultLiquidityCap),
				},
			},
		},
		{
			name: "invalid quote denom -> zero capitalization & error set",

			poolIDs: map[uint64]struct{}{
				defaultPoolID: {},
			},

			existingPools:     []sqsdomain.PoolI{&mocks.MockRoutablePool{ID: defaultPoolID, Balances: sdk.NewCoins(defaultUOSMOBalance)}},
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        ATOM,

			expectedLiquidityResultByID: map[uint64]liquidityResult{
				defaultPoolID: {
					LiquidityCap:      zeroCapitalization,
					LiquidityCapError: worker.FormatLiquidityCapErrorStr(UOSMO),
				},
			},
		},
		{
			name: "empty price -> zero capitalization & error set",

			poolIDs: map[uint64]struct{}{
				defaultPoolID: {},
			},

			existingPools:     []sqsdomain.PoolI{&mocks.MockRoutablePool{ID: defaultPoolID, Balances: sdk.NewCoins(defaultUOSMOBalance)}},
			blockPriceUpdates: domain.PricesResult{},
			quoteDenom:        USDC,

			expectedLiquidityResultByID: map[uint64]liquidityResult{
				defaultPoolID: {
					LiquidityCap:      zeroCapitalization,
					LiquidityCapError: worker.FormatLiquidityCapErrorStr(UOSMO),
				},
			},
		},
		{
			name: "empty pool IDs -> zero capitalization",

			poolIDs: map[uint64]struct{}{},

			existingPools:     []sqsdomain.PoolI{&mocks.MockRoutablePool{ID: defaultPoolID, Balances: sdk.NewCoins(defaultUOSMOBalance), PoolLiquidityCap: zeroCapitalization}},
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			expectedLiquidityResultByID: map[uint64]liquidityResult{
				defaultPoolID: {
					LiquidityCap: zeroCapitalization,
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

			// Create liquidity pricer
			liquidityPricer := worker.NewLiquidityPricer(tt.quoteDenom, mocks.SetupMockScalingFactorCbFromMap(defaultScalingFactorMap))

			// Create pool handler mock
			poolHandlerMock := &mocks.PoolHandlerMock{
				Pools: tt.existingPools,
			}

			// Create the worker
			poolLiquidityPricerWorker := worker.NewPoolLiquidityWorker(nil, poolHandlerMock, liquidityPricer, &log.NoOpLogger{})

			// System under test
			err := poolLiquidityPricerWorker.RepricePoolLiquidityCap(tt.poolIDs, tt.blockPriceUpdates)

			// Check the result
			if tt.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tt.expectError)
				return
			}

			// Convert all pools pre-set in state to the pool IDs
			// used for assertions.
			expectedPoolIDs := make([]uint64, 0, len(tt.existingPools))
			for _, pool := range tt.existingPools {
				expectedPoolIDs = append(expectedPoolIDs, pool.GetId())
			}

			// Get pools
			actualPools, _, err := poolHandlerMock.GetPools(domain.WithPoolIDFilter(expectedPoolIDs))
			s.Require().Equal(len(tt.expectedLiquidityResultByID), len(poolHandlerMock.Pools))

			// Validate that liquidity cap is set correctly on each pool
			s.validateLiquidityCapPools(tt.expectedLiquidityResultByID, actualPools)
		})
	}
}

// validatePoolDenomMetadata validates the pool denom metadata map.
func (s *PoolLiquidityComputeWorkerSuite) validatePoolDenomMetadata(expected domain.PoolDenomMetaDataMap, actual domain.PoolDenomMetaDataMap) {
	s.Require().Equal(len(expected), len(actual))
	for denom, expectedDenomMetadata := range expected {
		actualDenomMetadata, ok := actual[denom]
		s.Require().True(ok)

		s.Require().Equal(expectedDenomMetadata, actualDenomMetadata)
	}
}

// validateLiquidityCapPools validate that liquidity cap is set correctly on each pool.
func (s *PoolLiquidityComputeWorkerSuite) validateLiquidityCapPools(expectedLiquidityResultMap map[uint64]liquidityResult, actualPools []sqsdomain.PoolI) {
	for _, pool := range actualPools {

		expectedLiquidityResult, ok := expectedLiquidityResultMap[pool.GetId()]
		s.Require().True(ok)

		s.Require().Equal(expectedLiquidityResult.LiquidityCap, pool.GetLiquidityCap())
		s.Require().Equal(expectedLiquidityResult.LiquidityCapError, pool.GetLiquidityCapError())
	}
}
