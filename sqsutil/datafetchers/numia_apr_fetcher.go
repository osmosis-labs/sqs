package datafetchers

import (
	"github.com/osmosis-labs/sqs/domain"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// GetFetchPoolAPRsFromNumiaCb returns a callback to fetch pool APRs from Numia.
// It increments the error counter if the pool APRs fetching fails.
// It returns a callback function that returns the pool APRs on success.
func GetFetchPoolAPRsFromNumiaCb(numiaHTTPClient passthroughdomain.NumiaHTTPClient, logger log.Logger) func() []passthroughdomain.PoolAPR {
	return func() []passthroughdomain.PoolAPR {
		// Fetch pool APRs from the passthrough grpc client
		poolAPRs, err := numiaHTTPClient.GetPoolAPRsRange()
		if err != nil {
			logger.Error("Failed to fetch pool APRs", zap.Error(err))

			// Increment the error counter
			domain.SQSPassthroughNumiaAPRsFetchErrorCounter.Inc()
		}
		return poolAPRs
	}
}

// GetFetchPoolPoolFeesFromTimeseries returns a callback to fetch pool fees from timeseries data stack.
// It increments the error counter if the pool fees fetching fails.
// It returns a callback function that returns the pool fees on success.
func GetFetchPoolPoolFeesFromTimeseries(timeseriesHTTPClient passthroughdomain.TimeSeriesHTTPClient, logger log.Logger) func() map[string]passthroughdomain.PoolFee {
	return func() map[string]passthroughdomain.PoolFee {
		// Fetch pool APRs from the passthrough grpc client
		poolFees, err := timeseriesHTTPClient.GetPoolFees()
		if err != nil {
			logger.Error("Failed to fetch pool fees", zap.Error(err))

			// Increment the error counter
			domain.SQSPassthroughTimeseriesPoolFeesFetchErrorCounter.Inc()
		}

		poolFeesMap := make(map[string]passthroughdomain.PoolFee, len(poolFees.Data))
		for _, poolFee := range poolFees.Data {
			poolFeesMap[poolFee.PoolID] = poolFee
		}

		return poolFeesMap
	}
}
