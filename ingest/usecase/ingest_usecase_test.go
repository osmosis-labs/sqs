package usecase_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

var (
	UOSMO = routertesting.UOSMO
	USDC  = routertesting.USDC
)

type IngestUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

func TestIngestUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(IngestUseCaseTestSuite))
}
