package usecase_test

import (
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/ingest/types/cosmwasmpool"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/ingest/usecase"
)

var (
	tenE6  = osmomath.NewInt(10).ToLegacyDec().Power(6).TruncateInt()
	tenE8  = osmomath.NewInt(10).ToLegacyDec().Power(8).TruncateInt()
	tenE12 = osmomath.NewInt(10).ToLegacyDec().Power(12).TruncateInt()
	tenE9  = osmomath.NewInt(10).ToLegacyDec().Power(9).TruncateInt()
	tenE10 = osmomath.NewInt(10).ToLegacyDec().Power(10).TruncateInt()
	tenE18 = osmomath.NewInt(10).ToLegacyDec().Power(18).TruncateInt()
)

func (s *IngestUseCaseTestSuite) TestProcessAlloyedPool() {
	sqsModel := &ingesttypes.SQSPool{
		CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{
			Data: cosmwasmpool.CosmWasmPoolData{
				AlloyTransmuter: &cosmwasmpool.AlloyTransmuterData{
					AlloyedDenom: ALLBTC,
					AssetConfigs: []cosmwasmpool.TransmuterAssetConfig{
						{
							Denom:               ALLUSDT,
							NormalizationFactor: tenE18,
						},
						{
							Denom:               USDC,
							NormalizationFactor: tenE6,
						},
					},
				},
			},
		},
	}

	expectedPreComputedData := cosmwasmpool.PrecomputedData{
		StdNormFactor: tenE18,
		NormalizationScalingFactors: map[string]osmomath.Int{
			ALLUSDT: oneInt,
			USDC:    tenE12,
		},
	}

	// System under test
	err := usecase.ProcessAlloyedPool(sqsModel)
	s.Require().NoError(err)

	s.Require().Equal(expectedPreComputedData, sqsModel.CosmWasmPoolModel.Data.AlloyTransmuter.PreComputedData)
}

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

			normalizationFactor, err := usecase.ComputeStandardNormalizationFactor(tc.assetConfigs)

			if tc.expectedError {
				s.Require().Error(err)
				return
			}

			s.Require().Equal(tc.expected, normalizationFactor)
		})
	}
}

func (s *IngestUseCaseTestSuite) TestComputeNormalizationScalingFactors() {
	testCases := []struct {
		name string

		standardNormalizationFactor osmomath.Int
		assetConfigs                []cosmwasmpool.TransmuterAssetConfig

		expected      map[string]osmomath.Int
		expectedError bool
	}{
		{
			name: "empty",

			standardNormalizationFactor: tenE6,
			assetConfigs:                []cosmwasmpool.TransmuterAssetConfig{},

			expected: map[string]osmomath.Int{},
		},
		{
			name: "one asset",

			standardNormalizationFactor: tenE6,
			assetConfigs: []cosmwasmpool.TransmuterAssetConfig{
				{
					Denom:               ATOM,
					NormalizationFactor: tenE6,
				},
			},

			expected: map[string]osmomath.Int{
				ATOM: osmomath.OneInt(),
			},
		},
		{
			name: "two assets",

			standardNormalizationFactor: tenE18,
			assetConfigs: []cosmwasmpool.TransmuterAssetConfig{
				{
					Denom:               ATOM,
					NormalizationFactor: tenE6,
				},
				{
					Denom:               ALLBTC,
					NormalizationFactor: tenE8,
				},
			},

			expected: map[string]osmomath.Int{
				ATOM:   tenE12,
				ALLBTC: tenE10,
			},
		},
		{
			name:          "no standard normalization factor",
			assetConfigs:  []cosmwasmpool.TransmuterAssetConfig{},
			expectedError: true,
		},
		{
			name:                        "no asset normalization factor",
			standardNormalizationFactor: tenE6,
			assetConfigs: []cosmwasmpool.TransmuterAssetConfig{
				{
					Denom: ATOM,
				},
			},

			expectedError: true,
		},

		{
			name: "truncates to zero",

			standardNormalizationFactor: tenE6,
			assetConfigs: []cosmwasmpool.TransmuterAssetConfig{
				{
					Denom:               ATOM,
					NormalizationFactor: tenE10,
				},
			},

			expectedError: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {

			normalizationFactors, err := usecase.ComputeNormalizationScalingFactors(tc.standardNormalizationFactor, tc.assetConfigs)

			if tc.expectedError {
				s.Require().Error(err)
				return
			}

			s.Require().Len(normalizationFactors, len(tc.expected))
			for k, v := range tc.expected {
				s.Require().Equal(v.String(), normalizationFactors[k].String())
			}
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
