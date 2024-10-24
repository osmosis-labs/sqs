package routerrepo_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// RouteRepositoryChatGPTTestSuite defines the suite for testing RouterRepository
// Generated using ChatGPT based on method specs.
type RouteRepositoryChatGPTTestSuite struct {
	suite.Suite
	repository routerrepo.RouterRepository
}

var (
	fee1 osmomath.Dec = osmomath.NewDec(5)
	fee2 osmomath.Dec = osmomath.NewDec(10)
)

// In order to run the suite, you'll need this Test function
func TestRouteRepositoryTestSuite(t *testing.T) {
	suite.Run(t, new(RouteRepositoryChatGPTTestSuite))
}

// SetupTest prepares the environment for each test
func (suite *RouteRepositoryChatGPTTestSuite) SetupTest() {
	suite.repository = routerrepo.New(&log.NoOpLogger{}) // Implement this function to instantiate your repository
}

// TestGetTakerFee tests the GetTakerFee method
func (suite *RouteRepositoryChatGPTTestSuite) TestGetTakerFee() {
	var someFee osmomath.Dec = osmomath.NewDec(5) // example fee, adjust as necessary

	tests := []struct {
		name        string
		denom0      string
		denom1      string
		setup       func()
		expectedFee osmomath.Dec
		expectedOk  bool
	}{
		{
			name:   "successful lookup with denominations in lexicographical order",
			denom0: "denomA",
			denom1: "denomB",
			setup: func() {
				suite.repository.SetTakerFee("denomA", "denomB", someFee)
			},
			expectedFee: someFee,
			expectedOk:  true,
		},
		{
			name:       "unsuccessful lookup",
			denom0:     "denomX",
			denom1:     "denomY",
			setup:      func() {},
			expectedOk: false,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

			fee, ok := suite.repository.GetTakerFee(tt.denom0, tt.denom1)
			assert.Equal(suite.T(), tt.expectedOk, ok)
			if ok {
				assert.True(suite.T(), fee.Equal(tt.expectedFee))
			}
		})
	}
}

func (suite *RouteRepositoryChatGPTTestSuite) TestGetAllTakerFees() {
	tests := []struct {
		name              string
		setup             func()
		expectedTakerFees sqsdomain.TakerFeeMap
	}{
		{
			name:              "no taker fees set",
			setup:             func() {}, // No setup needed as there are no fees set
			expectedTakerFees: sqsdomain.TakerFeeMap{},
		},
		{
			name: "taker fees set for multiple pairs",
			setup: func() {
				suite.repository.SetTakerFee("denomA", "denomB", fee1)
				suite.repository.SetTakerFee("denomC", "denomD", fee2)
			},
			expectedTakerFees: sqsdomain.TakerFeeMap{
				sqsdomain.DenomPair{Denom0: "denomA", Denom1: "denomB"}: fee1,
				sqsdomain.DenomPair{Denom0: "denomC", Denom1: "denomD"}: fee2,
			},
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

			takerFees := suite.repository.GetAllTakerFees()
			assert.Equal(suite.T(), tt.expectedTakerFees, takerFees)
		})
	}
}

func (suite *RouteRepositoryChatGPTTestSuite) TestSetTakerFee() {
	tests := []struct {
		name   string
		denom0 string
		denom1 string
		fee    osmomath.Dec
	}{
		{
			name:   "set a single taker fee",
			denom0: "denomE",
			denom1: "denomF",
			fee:    fee1,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repository.SetTakerFee(tt.denom0, tt.denom1, tt.fee)

			fee, ok := suite.repository.GetTakerFee(tt.denom0, tt.denom1)
			assert.True(suite.T(), ok)
			assert.True(suite.T(), fee.Equal(tt.fee))
		})
	}
}

func (suite *RouteRepositoryChatGPTTestSuite) TestSetTakerFees() {
	expectedFees := sqsdomain.TakerFeeMap{
		sqsdomain.DenomPair{Denom0: "denomG", Denom1: "denomH"}: fee1,
		sqsdomain.DenomPair{Denom0: "denomI", Denom1: "denomJ"}: fee2,
	}

	tests := []struct {
		name         string
		takerFees    sqsdomain.TakerFeeMap
		expectedFees sqsdomain.TakerFeeMap
	}{
		{
			name: "set multiple taker fees",
			takerFees: sqsdomain.TakerFeeMap{
				sqsdomain.DenomPair{Denom0: "denomG", Denom1: "denomH"}: fee1,
				sqsdomain.DenomPair{Denom0: "denomI", Denom1: "denomJ"}: fee2,
			},
			expectedFees: expectedFees,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			suite.repository.SetTakerFees(tt.takerFees)

			fees := suite.repository.GetAllTakerFees()
			assert.Equal(suite.T(), tt.expectedFees, fees)
		})
	}
}

// Sanity checks validating the implementation of the GetRankedPoolsByDenom method
func (suite *RouteRepositoryChatGPTTestSuite) TestGetRankedPoolsByDenom_HappyPath() {
	const (
		defaultPoolID = 1

		denomA = "denomA"
		denomB = "denomB"

		denomNoPools = "denomNoPools"
	)

	var (
		denomOnePools = []sqsdomain.PoolI{
			&sqsdomain.PoolWrapper{
				ChainModel: &mocks.ChainPoolMock{
					ID: defaultPoolID,
				},
			}}

		denomTwoPools = []sqsdomain.PoolI{
			&sqsdomain.PoolWrapper{
				ChainModel: &mocks.ChainPoolMock{
					ID: defaultPoolID + 1,
				},
			}}
	)

	candidateRouteSearchData := map[string]domain.CandidateRouteDenomData{
		denomA: {
			SortedPools: denomOnePools,
		},
		denomB: {
			SortedPools: denomTwoPools,
		},
	}

	// System under test.
	suite.repository.SetCandidateRouteSearchData(candidateRouteSearchData)

	// Denom a has the expected pools.
	actualDenomOnePools, err := suite.repository.GetDenomData(denomA)
	suite.Require().NoError(err)
	suite.Require().Equal(denomOnePools, actualDenomOnePools.SortedPools)

	// Denom b has the expected pools.
	actualDenomTwoPools, err := suite.repository.GetDenomData(denomB)
	suite.Require().NoError(err)
	suite.Require().Equal(denomTwoPools, actualDenomTwoPools.SortedPools)

	// Denom with no pools returns an empty slice.
	actualNoDenomPools, err := suite.repository.GetDenomData(denomNoPools)
	suite.Require().NoError(err)
	suite.Require().Empty(actualNoDenomPools.SortedPools)
}
