package mocks

import (
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

// setupMockScalingFactorCb configures a mock scaling factor callback to return the given
// scaling factor and error.
// It also validates that mock receives valid parameters as given by validDenom.
// If validation does not pass, an error is returned rather than the mocked values.
func SetupMockScalingFactorCb(validDenom string, mockScalingFactor osmomath.Dec, mockError error) domain.ScalingFactorGetterCb {
	return func(denom string) (osmomath.Dec, error) {
		// Validate denom s equl to the one set on mock.
		if validDenom != denom {
			return osmomath.Dec{}, fmt.Errorf("actual  denom (%s) is not equal to the one configured by mock (%s)", denom, validDenom)
		}

		if mockScalingFactor.IsNil() {
			return osmomath.Dec{}, fmt.Errorf("scaling factor is nil for denom (%s)", validDenom)
		}

		return mockScalingFactor, mockError
	}
}

func SetupMockScalingFactorCbFromMap(scalingFactorMap map[string]osmomath.Dec) domain.ScalingFactorGetterCb {
	return func(denom string) (osmomath.Dec, error) {
		scalingFactor, ok := scalingFactorMap[denom]
		if !ok {
			return osmomath.Dec{}, fmt.Errorf("scaling factor for denom (%s) not found", denom)
		}

		return scalingFactor, nil
	}
}
