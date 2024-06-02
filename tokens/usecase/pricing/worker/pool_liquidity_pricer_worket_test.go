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

		zeroCapitalization = osmomath.ZeroInt()
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
