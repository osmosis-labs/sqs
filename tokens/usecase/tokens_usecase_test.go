package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/tokens/usecase"
)

type TokensUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultCosmosExponent = 6
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

	const (
		assetListFileURL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/osmosis-1.assetlist.json"
	)
	tokensMap, err := usecase.GetTokensFromChainRegistry(assetListFileURL)
	s.Require().NoError(err)
	s.Require().NotEmpty(tokensMap)

	// ATOM is present
	atomMainnetDenom := "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2"
	atomToken, ok := tokensMap[atomMainnetDenom]
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
		assetListFileURL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmo-test-5/osmo-test-5.assetlist.json"
	)
	tokensMap, err := usecase.GetTokensFromChainRegistry(assetListFileURL)
	s.Require().NoError(err)
	s.Require().NotEmpty(tokensMap)

	// uosmo is present
	uosmoDenom := "uosmo"
	osmoToken, ok := tokensMap[uosmoDenom]
	s.Require().True(ok)
	s.Require().Equal(defaultCosmosExponent, osmoToken.Precision)
}

func (s *TokensUseCaseTestSuite) TestGetPrices() {
	_, _, _ = s.SetupDefaultMainnetRouter()
}
