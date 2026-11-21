package pools_test

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"

	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"

	"github.com/osmosis-labs/osmosis/v31/app/apptesting"

	"github.com/stretchr/testify/suite"
)

type CosmWasmPoolSuite struct {
	apptesting.KeeperTestHelper
}

func TestCosmWasmPoolSuite(t *testing.T) {
	suite.Run(t, new(CosmWasmPoolSuite))
}

func (s *CosmWasmPoolSuite) SetupTest() {
	s.Setup()
}

func (s *CosmWasmPoolSuite) newPool(method domain.TokenSwapMethod, coin sdk.Coin, denom string, isInvalidPoolType bool, takerFee osmomath.Dec, err error) domain.RoutablePool {
	cosmwasmPool := s.PrepareCustomTransmuterPoolCustomProject(s.TestAccs[0], []string{coin.Denom, denom}, "sqs", "scripts")

	mock := &mocks.MockRoutablePool{ChainPoolModel: cosmwasmPool.AsSerializablePool(), PoolType: poolmanagertypes.CosmWasm}
	wasmclient := &mocks.WasmClient{}

	token := "token_out"
	if method == domain.TokenSwapMethodExactOut {
		token = "token_in"
	}
	wasmclient.WithSmartContractState(
		[]byte(fmt.Sprintf(`{ "%s": { "denom" : "%s", "amount" : "%s" } }`, token, ETH, coin.Amount.String())),
		err,
	)

	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		Config: domain.CosmWasmPoolRouterConfig{
			GeneralCosmWasmCodeIDs: map[uint64]struct{}{
				cosmwasmPool.GetCodeId(): {},
			},
		},
		WasmClient:            wasmclient,
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}

	routablePool, err := pools.NewRoutablePool(mock, coin.Denom, denom, takerFee, cosmWasmPoolsParams)
	s.Require().NoError(err)

	// Overwrite pool type for edge case testing
	if isInvalidPoolType {
		mock.PoolType = poolmanagertypes.Concentrated
	}

	return routablePool
}

// TestCalculateTokenInByTokenOut checks that a generalized CosmWasm pool refuses exact out, since such a
// pool can deliver less than the requested token out.
func (s *CosmWasmPoolSuite) TestCalculateTokenInByTokenOut() {
	s.Setup()

	tokenOut := sdk.NewCoin(USDC, DefaultAmt0)
	routablePool := s.newPool(domain.TokenSwapMethodExactOut, tokenOut, ETH, false, noTakerFee, nil)

	_, err := routablePool.CalculateTokenInByTokenOut(context.TODO(), tokenOut)
	s.Require().ErrorIs(err, domain.GeneralizedCosmWasmPoolExactOutError{PoolId: routablePool.GetId()})
}
