package worker_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
	"github.com/stretchr/testify/suite"
)

type PoolLiquidityComputeWorkerSuite struct {
	routertesting.RouterTestHelper
}

var (
	defaultPricingRouterConfig = domain.RouterConfig{
		PreferredPoolIDs:    []uint64{},
		MaxRoutes:           5,
		MaxPoolsPerRoute:    3,
		MaxSplitRoutes:      3,
		MinPoolLiquidityCap: 50,
		RouteCacheEnabled:   true,
	}

	pricingCacheExpiry = 2000

	defaultScalingFactor = osmomath.NewDec(1_000_000)

	zeroCapitalization = osmomath.ZeroInt()

	zeroPrice = osmomath.ZeroBigDec()
)

var (
	stableCoinDenoms = []string{"usdc", "usdt", "dai", "ist"}
)

func TestPoolLiquidityComputeWorkerSuite(t *testing.T) {
	suite.Run(t, new(PoolLiquidityComputeWorkerSuite))
}

func (s *PoolLiquidityComputeWorkerSuite) TestOnPricingUpdate() {

	tests := []struct {
		name string
	}{}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

		})
	}
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

// TestStoreHeightForDenom tests the StoreHeightForDenom method by following the spec.
func (s *PoolLiquidityComputeWorkerSuite) TestComputeLiquidityCapitalization() {
	var (
		defaultScalingFactorMap = map[string]osmomath.Dec{
			UOSMO: defaultScalingFactor,
		}

		defaultLiqidity = osmomath.NewInt(1_000_000)

		ethScaledLiquidity = ethScalingFactor.MulInt(defaultLiqidity).TruncateInt()

		defaultPriceOne = osmomath.OneBigDec()
	)

	tests := []struct {
		name string

		preSetScalingFactorMap map[string]osmomath.Dec

		denom          string
		totalLiquidity osmomath.Int
		price          osmomath.BigDec

		expectedCapitalization osmomath.Int
	}{
		{
			name: "scaling factor unset",

			preSetScalingFactorMap: map[string]osmomath.Dec{},

			denom:          UOSMO,
			totalLiquidity: defaultLiqidity,
			price:          defaultPriceOne,

			expectedCapitalization: zeroCapitalization,
		},
		{
			name: "zero price -> produces zero capitalization",

			preSetScalingFactorMap: defaultScalingFactorMap,

			denom:          UOSMO,
			totalLiquidity: defaultLiqidity,
			price:          osmomath.ZeroBigDec(),

			expectedCapitalization: zeroCapitalization,
		},
		{
			name: "truncate -> produces zero capitalization",

			// totalLiquidity * price / (quoteScalingFactor / baseScalingFactor)
			// 1 * 10^-36 / 10^12 => below the precision of 36
			preSetScalingFactorMap: map[string]osmomath.Dec{
				UOSMO: ethScalingFactor,
			},

			denom:          UOSMO,
			totalLiquidity: osmomath.OneInt(),
			price:          osmomath.SmallestBigDec(),

			expectedCapitalization: zeroCapitalization,
		},
		{
			name: "happy path",

			preSetScalingFactorMap: defaultScalingFactorMap,

			denom:          UOSMO,
			totalLiquidity: defaultLiqidity,
			price:          defaultPriceOne,

			expectedCapitalization: defaultLiqidity,
		},
		{
			name: "happy path with different inputs",

			preSetScalingFactorMap: map[string]osmomath.Dec{
				ATOM: ethScalingFactor,
			},

			denom:          ATOM,
			totalLiquidity: ethScaledLiquidity.MulRaw(2),
			price:          osmomath.NewBigDec(2),

			expectedCapitalization: ethScaledLiquidity.ToLegacyDec().MulMut(defaultScalingFactor).QuoMut(ethScalingFactor).TruncateInt().MulRaw(4),
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {
			// Create liquidity pricer
			liquidityPricer := worker.NewLiquidityPricer(USDC, defaultQuoteDenomScalingFactor)

			// Set up the tokens pool liquidity mock handler
			poolLiquidityHandlerMock := mocks.TokensPoolLiquidityHandlerMock{
				DenomScalingFactorMap: tt.preSetScalingFactorMap,
			}

			// Create the worker
			poolLiquidityPricerWorker := worker.NewPoolLiquidityWorker(&poolLiquidityHandlerMock, liquidityPricer)

			// System under test
			liquidityCapitalization := poolLiquidityPricerWorker.ComputeLiquidityCapitalization(tt.denom, tt.totalLiquidity, tt.price)

			// Check the result
			s.Require().Equal(tt.expectedCapitalization.String(), liquidityCapitalization.String())
		})
	}
}

// TestStoreHeightForDenom tests the StoreHeightForDenom method by following the spec.
func (s *PoolLiquidityComputeWorkerSuite) TestRepriceDenomMetadata() {
	const (
		defaultUpdateHeight uint64 = 2
	)

	var (
		// Note: we are not testing the error handling of underlying methods.
		// Those are unit-tested in their respective tests.
		// As a result, we only set up the valid cases here.
		defaultScalingFactorMap = map[string]osmomath.Dec{
			UOSMO: defaultScalingFactor,
			USDC:  defaultScalingFactor,
			ATOM:  defaultScalingFactor,
		}

		defaultPrice     = osmomath.NewBigDec(2)
		defaultLiquidity = osmomath.NewInt(1_000_000)

		defaultLiquidityCap = defaultLiquidity.MulRaw(2)

		defaultBlockPriceUpdates = domain.PricesResult{
			UOSMO: {
				USDC: defaultPrice,
			},
		}

		defaultBlockLiquidityUpdates = domain.DenomLiquidityMap{
			UOSMO: {
				TotalLiquidity: defaultLiquidity,
			},
		}

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
		defaultZeroPricePoolDenomMetaDattaMapResult = domain.PoolDenomMetaDataMap{
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

		updateHeight                  uint64
		blockPriceUpdates             domain.PricesResult
		quoteDenom                    string
		blockDenomLiquidityUpdatesMap domain.DenomLiquidityMap

		expectedUpdatedDenomMetadata domain.PoolDenomMetaDataMap

		expectedDenomHeights map[string]uint64
	}{
		{
			name: "one denom success case",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			blockDenomLiquidityUpdatesMap: defaultBlockLiquidityUpdates,

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

			blockDenomLiquidityUpdatesMap: domain.DenomLiquidityMap{},
			expectedUpdatedDenomMetadata:  domain.PoolDenomMetaDataMap{},

			expectedDenomHeights: zeroUOSMOHeightResult,
		},

		{
			name: "one denom - later update exists the height and metadata are not updated",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			quoteDenom:        USDC,

			// Note: pre-set to this.
			preSetUpdateHeightForDenom: laterUOSMOHeightResult,

			blockDenomLiquidityUpdatesMap: defaultBlockLiquidityUpdates,

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

			blockDenomLiquidityUpdatesMap: defaultBlockLiquidityUpdates,

			// Note: zero price result.
			expectedUpdatedDenomMetadata: defaultZeroPricePoolDenomMetaDattaMapResult,

			expectedDenomHeights: defaultUOSMOHeightResult,
		},

		{
			name: "one denom, the quote denom is for a different price, setting the pool liquidity capitalization to zero",

			updateHeight:      defaultUpdateHeight,
			blockPriceUpdates: defaultBlockPriceUpdates,
			// Note: different quote denom
			quoteDenom: ATOM,

			// Note: zero price result.
			blockDenomLiquidityUpdatesMap: defaultBlockLiquidityUpdates,

			expectedUpdatedDenomMetadata: defaultZeroPricePoolDenomMetaDattaMapResult,

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

			blockDenomLiquidityUpdatesMap: domain.DenomLiquidityMap{
				UOSMO: {
					TotalLiquidity: defaultLiquidity,
				},
				ATOM: {
					// 2x the liquidity
					TotalLiquidity: defaultLiquidity.Add(defaultLiquidity),
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
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

			// Create liquidity pricer
			liquidityPricer := worker.NewLiquidityPricer(USDC, defaultQuoteDenomScalingFactor)

			// Set up the tokens pool liquidity mock handler
			poolLiquidityHandlerMock := mocks.TokensPoolLiquidityHandlerMock{
				DenomScalingFactorMap: defaultScalingFactorMap,
			}

			// Create the worker
			poolLiquidityPricerWorker := worker.NewPoolLiquidityWorker(&poolLiquidityHandlerMock, liquidityPricer)

			// Pre-set the height for each denom.
			for denom, height := range tt.preSetUpdateHeightForDenom {
				poolLiquidityPricerWorker.StoreHeightForDenom(denom, height)
			}

			// System under test
			poolDenomMetadata := poolLiquidityPricerWorker.RepriceDenomMetadata(tt.updateHeight, tt.blockPriceUpdates, tt.quoteDenom, tt.blockDenomLiquidityUpdatesMap)

			// Check the result
			// TODO: move into function
			s.validatePoolDenomMetadata(tt.expectedUpdatedDenomMetadata, poolDenomMetadata)

			// Check the height is stored correctly
			for denom, expectedHeight := range tt.expectedDenomHeights {
				actualHeight := poolLiquidityPricerWorker.GetHeightForDenom(denom)

				s.Require().Equal(expectedHeight, actualHeight)
			}
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
