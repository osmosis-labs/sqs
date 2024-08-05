package usecase_test

import (
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

var (
	tenE6  = osmomath.NewInt(10).ToLegacyDec().Power(6).TruncateInt()
	tenE8  = osmomath.NewInt(10).ToLegacyDec().Power(8).TruncateInt()
	tenE12 = osmomath.NewInt(10).ToLegacyDec().Power(12).TruncateInt()
	tenE9  = osmomath.NewInt(10).ToLegacyDec().Power(9).TruncateInt()
	tenE18 = osmomath.NewInt(10).ToLegacyDec().Power(18).TruncateInt()
)

func (s *IngestUseCaseTestSuite) TestComputeStandardNormalizationFactor() {
	testCases := []struct {
		name         string
		assetConfigs []cosmwasmpool.TransmuterAssetConfig

		expected      osmomath.Int
		expectedError bool
	}{
		{
			name: "empty",

			assetConfigs: []cosmwasmpool.TransmuterAssetConfig{},

			expected: osmomath.OneInt(),
		},
		{
			name: "single",

			assetConfigs: []cosmwasmpool.TransmuterAssetConfig{
				{
					Denom:               ATOM,
					NormalizationFactor: tenE6,
				},
			},

			expected: tenE6,
		},

		{
			name: "multiple",

			assetConfigs: []cosmwasmpool.TransmuterAssetConfig{
				{
					Denom:               ATOM,
					NormalizationFactor: tenE6,
				},
				{
					Denom:               UOSMO,
					NormalizationFactor: tenE8,
				},
				{
					Denom:               USDC,
					NormalizationFactor: tenE12,
				},
				{
					Denom:               ALLBTC,
					NormalizationFactor: tenE9,
				},
				{
					Denom:               ALLUSDT,
					NormalizationFactor: tenE18,
				},
			},

			expected: tenE18,
		},
		{
			name: "no normalization factor",

			assetConfigs: []cosmwasmpool.TransmuterAssetConfig{
				{
					Denom: ATOM,
				},
			},

			expectedError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {

			normalizetionFactor, error := usecase.ComputeStandardNormalizationFactor(tc.assetConfigs)

			if tc.expectedError {
				s.Require().Error(error)
				return
			}

			s.Require().Equal(tc.expected, normalizetionFactor)
		})
	}
}

func (s *IngestUseCaseTestSuite) TestLcm() {
	a := osmomath.NewInt(10).ToLegacyDec().Power(18).TruncateInt()
	b := osmomath.NewInt(10).ToLegacyDec().Power(12).TruncateInt()

	lcm := usecase.Lcm(a.BigInt(), b.BigInt())

	lcmInt := osmomath.NewIntFromBigInt(lcm)

	s.Require().Equal(osmomath.NewInt(10).ToLegacyDec().Power(18).TruncateInt(), lcmInt)
}
