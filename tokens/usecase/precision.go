package usecase

import "github.com/osmosis-labs/osmosis/osmomath"

var (
	tenDec = osmomath.NewDec(10)
	// No mutex since we only instantiate this once, and its static content
	precisionScalingFactors []osmomath.Dec
)

func init() {
	// Initialize the precision scaling factors
	precisionScalingFactors = buildPrecisionScalingFactors()
}

const maxDecPrecision = 74

func buildPrecisionScalingFactors() []osmomath.Dec {
	precisionScalingFactors := make([]osmomath.Dec, maxDecPrecision)
	for i := 0; i < 74; i++ {
		precisionScalingFactors[i] = tenDec.Power(uint64(i))
	}
	return precisionScalingFactors
}

func getPrecisionScalingFactorMut(precision int) (osmomath.Dec, bool) {
	if precision < 0 || precision >= len(precisionScalingFactors) {
		return osmomath.Dec{}, false
	}
	result := precisionScalingFactors[precision]
	return result, true
}
