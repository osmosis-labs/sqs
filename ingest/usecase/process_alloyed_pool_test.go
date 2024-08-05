package usecase_test

import (
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/ingest/usecase"
)

func (s *IngestUseCaseTestSuite) TestLcm() {
	a := osmomath.NewInt(10).ToLegacyDec().Power(18).TruncateInt()
	b := osmomath.NewInt(10).ToLegacyDec().Power(12).TruncateInt()

	lcm := usecase.Lcm(a.BigInt(), b.BigInt())

	lcmInt := osmomath.NewIntFromBigInt(lcm)

	s.Require().Equal(osmomath.NewInt(10).ToLegacyDec().Power(18).TruncateInt(), lcmInt)
}
