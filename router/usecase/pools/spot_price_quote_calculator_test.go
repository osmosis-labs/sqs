package pools_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
)

// This test case validates that the spot price quote calculator computes
// the spot price via quote as intended and returns errors in edge cases
// as given by the spec.
//
// To setup the environment, it configures 2 mocks:
// 1. Mock quote estimator callback
// 2. Scaling factor computer callback.
//
// Then, by using the mocked outputs, it validates the behavior to be as intended.
// For details about mock configuration, read their respective setup functions.
func (s *RoutablePoolTestSuite) TestSpotPriceQuoteCalculator_Calculate() {
	// setupMockQuoteEstimator configures a mock quote estimator using the given
	// coin and error.
	// It also validates that mock receives valid parameters as given by validTokenIn and validTokenOutDenom.
	// If validation does not pass, an error is returned rather than the mocked values.
	setupMockQuoteEstimator := func(validTokenIn sdk.Coin, validTokenOutDenom string, mockCoinOutput sdk.Coin, mockError error) domain.QuoteEstimatorCb {
		return func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (sdk.Coin, error) {

			// Validate token in is equal to the one set on mock.
			if !tokenIn.Equal(validTokenIn) {
				return sdk.Coin{}, fmt.Errorf("actual token in (%s) is not equal to the one configured by mock (%s)", tokenIn, validTokenIn)
			}

			// Validate token out denom is equal to the one set on mock.
			if tokenOutDenom != validTokenOutDenom {
				return sdk.Coin{}, fmt.Errorf("actual token out denom (%s) is not equal to the one configured by mock (%s)", tokenOutDenom, validTokenOutDenom)
			}

			// If validation passes, return the desired outputs.s
			return mockCoinOutput, mockError
		}
	}

	// setupMockScalingFactorCb configures a mock scaling factor callback to return the given
	// scaling factor and error.
	// It also validates that mock receives valid parameters as given by validDenom.
	// If validation does not pass, an error is returned rather than the mocked values.
	setupMockScalingFactorCb := func(validDenom string, mockScalingFactor osmomath.Dec, mockError error) domain.ScalingFactorGetterCb {
		return func(denom string) (osmomath.Dec, error) {
			// Validate denom s equl to the one set on mock.
			if validDenom != denom {
				return osmomath.Dec{}, fmt.Errorf("actual  denom (%s) is not equal to the one configured by mock (%s)", denom, validDenom)
			}

			return mockScalingFactor, mockError
		}
	}

	var (
		defaultQuoteDenom = routertesting.UOSMO
		defaultBaseDenom  = routertesting.USDC

		defaultAmountOut       = osmomath.NewInt(123_456_789)
		defaultCoinOutEstimate = sdk.NewCoin(defaultBaseDenom, defaultAmountOut)

		// 10e6
		defaultScalingFactor = osmomath.NewDec(1_000_000)
		// 10e19
		tenE18 = osmomath.NewInt(1_000_000_000_000_000_000)
		tenE37 = tenE18.Mul(tenE18).MulRaw(10)

		defaultCoin = sdk.NewCoin(defaultQuoteDenom, defaultScalingFactor.TruncateInt())

		defaultExpectedSpotPrice = osmomath.BigDecFromDec(defaultScalingFactor).Quo(osmomath.BigDecFromSDKInt(defaultAmountOut))

		mockError = errors.New("mock error")
	)

	tests := []struct {
		name                string
		mockCoinOutEstimate sdk.Coin
		mockCoinOutError    error

		mockScalingFactor      osmomath.Dec
		mockScalingFactorError error

		baseDenom  string
		quoteDenom string

		expectedSpotPrice osmomath.BigDec
		expectedError     error
	}{
		{
			name:                "happy path",
			mockCoinOutEstimate: defaultCoinOutEstimate,

			mockScalingFactor: defaultScalingFactor,

			baseDenom:  defaultBaseDenom,
			quoteDenom: defaultQuoteDenom,

			expectedSpotPrice: defaultExpectedSpotPrice,
		},
		{
			name:                "error: fails to retrieve scaling factor for the quote denom",
			mockCoinOutEstimate: defaultCoinOutEstimate,

			// Note: we use scaling factor for pre-setting test case, that's why we need
			// to initialize it.
			mockScalingFactor: defaultScalingFactor,
			// Scaling factor getter is mocked to error.
			mockScalingFactorError: mockError,

			baseDenom:  defaultBaseDenom,
			quoteDenom: defaultQuoteDenom,

			expectedError: mockError,
		},
		{
			name: "error: quote fails to be computed.",
			// Estimate is mocked to error.
			mockCoinOutError: mockError,

			mockScalingFactor: defaultScalingFactor,

			baseDenom:  defaultBaseDenom,
			quoteDenom: defaultQuoteDenom,

			expectedError: mockError,
		},
		{
			name: "error: quote outputs zero amount",
			// Estimate is mocked to error.
			mockCoinOutEstimate: sdk.NewCoin(defaultBaseDenom, osmomath.ZeroInt()),

			mockScalingFactor: defaultScalingFactor,

			baseDenom:  defaultBaseDenom,
			quoteDenom: defaultQuoteDenom,

			expectedError: domain.SpotPriceQuoteCalculatorOutAmountZeroError{
				QuoteCoinStr: defaultCoin.String(),
				BaseDenom:    defaultBaseDenom,
			},
		},
		{
			name: "error: quote outputs nil coin",
			// Estimate is mocked to error.
			mockCoinOutEstimate: sdk.Coin{},

			mockScalingFactor: defaultScalingFactor,

			baseDenom:  defaultBaseDenom,
			quoteDenom: defaultQuoteDenom,

			expectedError: domain.SpotPriceQuoteCalculatorOutAmountZeroError{
				QuoteCoinStr: defaultCoin.String(),
				BaseDenom:    defaultBaseDenom,
			},
		},
		{
			name: "error: quote outputs coin with nil amoint",
			// Estimate is mocked to error.
			mockCoinOutEstimate: sdk.Coin{Denom: defaultBaseDenom, Amount: osmomath.Int{}},

			mockScalingFactor: defaultScalingFactor,

			baseDenom:  defaultBaseDenom,
			quoteDenom: defaultQuoteDenom,

			expectedError: domain.SpotPriceQuoteCalculatorOutAmountZeroError{
				QuoteCoinStr: defaultCoin.String(),
				BaseDenom:    defaultBaseDenom,
			},
		},
		{
			name: "error: truncation in intermediary calculations happens, leading to spot price of zero",
			// Estimate is mocked to error.
			mockCoinOutEstimate: sdk.NewCoin(defaultBaseDenom, tenE37),

			mockScalingFactor: osmomath.OneDec(),

			baseDenom:  defaultBaseDenom,
			quoteDenom: defaultQuoteDenom,

			// 1 / 10e37 = 10e-37 which is below the minimum big decimal
			expectedError: domain.SpotPriceQuoteCalculatorTruncatedError{
				QuoteCoinStr: sdk.NewCoin(defaultQuoteDenom, osmomath.OneInt()).String(),
				BaseDenom:    defaultBaseDenom,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		s.T().Run(tc.name, func(t *testing.T) {

			// Note: the quote should be done on the quote denom in with scaling factor as amount in
			validQuoteInputCoin := sdk.NewCoin(tc.quoteDenom, tc.mockScalingFactor.TruncateInt())
			// Respectively, the token out denom is the base denom.
			validQuoteInputTokenOutDenom := tc.baseDenom
			// And scaling factor denom is the quote denom by construction
			validScalingFactorDenom := tc.quoteDenom

			// Setup the mocks.
			mockQuoteEstimator := setupMockQuoteEstimator(validQuoteInputCoin, validQuoteInputTokenOutDenom, tc.mockCoinOutEstimate, tc.mockCoinOutError)
			mockScalingFactorGetter := setupMockScalingFactorCb(validScalingFactorDenom, tc.mockScalingFactor, tc.mockScalingFactorError)

			// Initialize spot price quote calculator.
			spotPriceQuoteCalculator := pools.NewSpotPriceQuoteComputer(mockScalingFactorGetter, mockQuoteEstimator)

			// System under test
			actualSpotPrice, err := spotPriceQuoteCalculator.Calculate(context.TODO(), tc.baseDenom, tc.quoteDenom)

			if tc.expectedError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectedError)
				return
			}

			s.Require().NoError(err)
			s.Require().Equal(tc.expectedSpotPrice, actualSpotPrice)
		})
	}
}
