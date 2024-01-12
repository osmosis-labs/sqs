package usecase_test

import (
	"context"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/pools/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	sqsdomainmocks "github.com/osmosis-labs/sqs/sqsdomain/mocks"
)

type PoolsUsecaseTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultPoolID = uint64(1)
)

var (
	denomOne   = routertesting.DenomOne
	denomTwo   = routertesting.DenomTwo
	denomThree = routertesting.DenomThree
	denomFour  = routertesting.DenomFour
	denomFive  = routertesting.DenomFive

	defaultTakerFee = routertesting.DefaultTakerFee

	defaultAmt0 = routertesting.DefaultAmt0
	defaultAmt1 = routertesting.DefaultAmt1
)

func TestPoolsUsecaseTestSuite(t *testing.T) {
	suite.Run(t, new(PoolsUsecaseTestSuite))
}

// Validates that candidate routes are correctly converted into routes with all the pool data.
// Check that:
// - pool data is correctly set on routable pools.
// - taker fee is correctly set.
// - token out denom is correctly set.
func (s *PoolsUsecaseTestSuite) TestGetRoutesFromCandidates() {

	s.Setup()

	// Setup default chain pool
	poolID := s.PrepareBalancerPoolWithCoins(sdk.NewCoin(denomOne, defaultAmt0), sdk.NewCoin(denomTwo, defaultAmt1))
	balancerPool, err := s.App.GAMMKeeper.GetPool(s.Ctx, poolID)
	s.Require().NoError(err)

	defaultPool := &mocks.MockRoutablePool{
		ChainPoolModel: balancerPool,
		ID:             defaultPoolID,
	}

	validPools := []sqsdomain.PoolI{
		defaultPool,
	}

	validCandidateRoutes := sqsdomain.CandidateRoutes{
		Routes: []sqsdomain.CandidateRoute{
			{
				Pools: []sqsdomain.CandidatePool{
					{
						ID:            defaultPoolID,
						TokenOutDenom: denomTwo,
					},
				},
			},
		},
	}

	validTakerFeeMap := sqsdomain.TakerFeeMap{
		sqsdomain.DenomPair{
			Denom0: denomOne,
			Denom1: denomTwo,
		}: defaultTakerFee,
	}

	tests := []struct {
		name string

		pools           []sqsdomain.PoolI
		candidateRoutes sqsdomain.CandidateRoutes
		takerFeeMap     sqsdomain.TakerFeeMap
		tokenInDenom    string
		tokenOutDenom   string

		expectedError error

		expectedRoutes []route.RouteImpl
	}{
		{
			name:  "valid conversion of single route",
			pools: validPools,

			candidateRoutes: validCandidateRoutes,
			takerFeeMap:     validTakerFeeMap,

			tokenInDenom:  denomOne,
			tokenOutDenom: denomTwo,

			expectedRoutes: []route.RouteImpl{
				{
					Pools: []sqsdomain.RoutablePool{
						s.newRoutablePool(defaultPool, denomTwo, defaultTakerFee, domain.CosmWasmCodeIDMaps{}),
					},
				},
			},
		},
		{
			name:  "no taker fee - use default",
			pools: validPools,

			candidateRoutes: validCandidateRoutes,

			// empty map
			takerFeeMap: sqsdomain.TakerFeeMap{},

			tokenInDenom:  denomOne,
			tokenOutDenom: denomTwo,

			expectedRoutes: []route.RouteImpl{
				{
					Pools: []sqsdomain.RoutablePool{
						s.newRoutablePool(defaultPool, denomTwo, sqsdomain.DefaultTakerFee, domain.CosmWasmCodeIDMaps{}),
					},
				},
			},
		},
		{
			name:  "error: no pool in state",
			pools: []sqsdomain.PoolI{},

			candidateRoutes: validCandidateRoutes,

			// empty map
			takerFeeMap: validTakerFeeMap,

			tokenInDenom:  denomOne,
			tokenOutDenom: denomTwo,

			expectedError: domain.PoolNotFoundError{
				PoolID: defaultPoolID,
			},
		},

		// TODO:
		// Valid conversion of single multi-hop route
		// Valid conversion of two routes where one is multi hop
	}

	for _, tc := range tests {
		tc := tc
		s.Run(tc.name, func() {

			// Create repository mock
			poolsRepository := &sqsdomainmocks.RedisPoolsRepositoryMock{
				Pools: tc.pools,
			}

			// Create pools use case
			poolsUsecase := usecase.NewPoolsUsecase(time.Second, poolsRepository, nil, &domain.PoolsConfig{})

			// System under test
			actualRoutes, err := poolsUsecase.GetRoutesFromCandidates(context.Background(), tc.candidateRoutes, tc.takerFeeMap, tc.tokenInDenom, tc.tokenOutDenom)

			if tc.expectedError != nil {
				s.Require().Error(err)
				s.Require().Equal(tc.expectedError, err)
				return
			}

			s.Require().NoError(err)

			// Validate routes
			s.Require().Equal(len(tc.expectedRoutes), len(actualRoutes))
			for i, actualRoute := range actualRoutes {
				expectedRoute := tc.expectedRoutes[i]

				// Note: this is only done to be able to use the ValidateRoutePools
				// helper method for validation.
				// Note token in is chosen arbitrarily since it is irrelevant for this test
				tokenIn := sdk.NewCoin(tc.tokenInDenom, sdk.NewInt(100))
				_, _, err := actualRoute.PrepareResultPools(context.TODO(), tokenIn)
				s.Require().NoError(err)
				_, _, err = expectedRoute.PrepareResultPools(context.TODO(), tokenIn)
				s.Require().NoError(err)

				// Validates:
				// 1. Correct pool data
				// 2. Correct taker fee
				// 3. Correct token out denom
				s.ValidateRoutePools(expectedRoute.Pools, actualRoute.Pools)
			}
		})
	}
}

func (s *PoolsUsecaseTestSuite) newRoutablePool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec, cosmWasmPoolIDs domain.CosmWasmCodeIDMaps) sqsdomain.RoutablePool {
	routablePool, err := pools.NewRoutablePool(pool, tokenOutDenom, takerFee, cosmWasmPoolIDs)
	s.Require().NoError(err)
	return routablePool
}
