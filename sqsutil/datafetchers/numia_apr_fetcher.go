package datafetchers

import (
	"strconv"

	"github.com/osmosis-labs/sqs/domain"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// GetFetchPoolAPRsFromNumiaCb returns a callback to fetch pool APRs from Numia.
// It increments the error counter if the pool APRs fetching fails.
// It returns a callback function that returns the pool APRs on success.
func GetFetchPoolAPRsFromNumiaCb(numiaHTTPClient passthroughdomain.NumiaHTTPClient, logger log.Logger) func() map[uint64]passthroughdomain.PoolAPR {
	return func() map[uint64]passthroughdomain.PoolAPR {
		// Fetch pool APRs from the passthrough grpc client
		poolAPRs, err := numiaHTTPClient.GetPoolAPRsRange()
		if err != nil {
			logger.Error("Failed to fetch pool APRs", zap.Error(err))

			// Increment the error counter
			domain.SQSPassthroughNumiaAPRsFetchErrorCounter.Inc()
		}

		// Convert to map
		poolAPRsMap := make(map[uint64]passthroughdomain.PoolAPR, len(poolAPRs))
		for _, poolAPR := range poolAPRs {
			poolAPRsMap[poolAPR.PoolID] = poolAPR
		}

		return poolAPRsMap
	}
}

// GetFetchPoolPoolFeesFromTimeseries returns a callback to fetch pool fees from timeseries data stack.
// It increments the error counter if the pool fees fetching fails.
// It returns a callback function that returns the pool fees on success.
func GetFetchPoolPoolFeesFromTimeseries(timeseriesHTTPClient passthroughdomain.TimeSeriesHTTPClient, logger log.Logger) func() map[uint64]passthroughdomain.PoolFee {
	return func() map[uint64]passthroughdomain.PoolFee {
		// Fetch pool APRs from the passthrough grpc client
		poolFees, err := timeseriesHTTPClient.GetPoolFees()
		if err != nil {
			logger.Error("Failed to fetch pool fees", zap.Error(err))

			// Increment the error counter
			domain.SQSPassthroughTimeseriesPoolFeesFetchErrorCounter.Inc()

			return nil
		}

		poolFeesMap := make(map[uint64]passthroughdomain.PoolFee, len(poolFees.Data))
		for _, poolFee := range poolFees.Data {
			// Convert pool ID to uint64
			poolID, err := strconv.ParseUint(poolFee.PoolID, 10, 64)
			if err != nil {
				logger.Error("Failed to parse pool ID", zap.Error(err))
				domain.SQSPassthroughTimeseriesPoolFeesFetchErrorCounter.Inc()
				continue
			}

			poolFeesMap[poolID] = poolFee
		}

		return poolFeesMap
	}
}
