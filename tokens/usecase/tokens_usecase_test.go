package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	tokensusecase "github.com/osmosis-labs/sqs/tokens/usecase"
)

type TokensUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultCosmosExponent = 6

	mainnetAssetListFileURL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/osmosis-1.assetlist.json"
)

var (
	UOSMO   = routertesting.UOSMO
	ATOM    = routertesting.ATOM
	stOSMO  = routertesting.STOSMO
	stATOM  = routertesting.STATOM
	USDC    = routertesting.USDC
	USDCaxl = routertesting.USDCaxl
	USDT    = routertesting.USDT
	WBTC    = routertesting.WBTC
	ETH     = routertesting.ETH
	AKT     = routertesting.AKT
	UMEE    = routertesting.UMEE
	UION    = routertesting.UION
)

func TestTokensUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(TokensUseCaseTestSuite))
}

func (s *TokensUseCaseTestSuite) TestParseExponents() {
	s.T().Skip("skip the test that does network call and is used for debugging")

	const ()
	tokensMap, err := tokensusecase.GetTokensFromChainRegistry(mainnetAssetListFileURL)
	s.Require().NoError(err)
	s.Require().NotEmpty(tokensMap)

	// ATOM is present
	atomToken, ok := tokensMap[ATOM]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, atomToken.Precision)

	// ION is present
	ionMainnetDenom := "uion"
	ionToken, ok := tokensMap[ionMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, ionToken.Precision)

	// IBCX is presnet
	ibcxMainnetDenom := "factory/osmo14klwqgkmackvx2tqa0trtg69dmy0nrg4ntq4gjgw2za4734r5seqjqm4gm/uibcx"
	ibcxToken, ok := tokensMap[ibcxMainnetDenom]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, ibcxToken.Precision)
}

func (s *TokensUseCaseTestSuite) TestParseExponents_Testnet() {
	s.T().Skip("skip the test that does network call and is used for debugging")

	const (
		testnetAssetListFileURL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmo-test-5/osmo-test-5.assetlist.json"
	)
	tokensMap, err := tokensusecase.GetTokensFromChainRegistry(testnetAssetListFileURL)
	s.Require().NoError(err)
	s.Require().NotEmpty(tokensMap)

	// uosmo is present
	osmoToken, ok := tokensMap[UOSMO]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, osmoToken.Precision)
}

func (s *TokensUseCaseTestSuite) TestGetPrices() {
	router, mainnetState := s.SetupDefaultMainnetRouter()

	// mock

	s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.NewNoOpRoutesOverwrite())

	// routerusecase.NewRouterUsecase(time.Second)

	// usecase.NewTokensUsecase(time.Second)
}
