package types

import (
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

func ValidateSimulationParams(swapMethod domain.TokenSwapMethod, simulatorAddress string, slippageToleranceStr string) (osmomath.Dec, error) {
	return validateSimulationParams(swapMethod, simulatorAddress, slippageToleranceStr)
}
