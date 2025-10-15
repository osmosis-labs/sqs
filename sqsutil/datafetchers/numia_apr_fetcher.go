package datafetchers

import (
	"fmt"
	"strconv"
	"time"

	sqspassthroughdomain "github.com/osmosis-labs/osmosis/v28/ingest/types/passthroughdomain"
	"github.com/osmosis-labs/sqs/domain"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// numiaAPRsFetchRetries is the number of retries to fetch pool APRs from Numia.
const (
	numiaAPRsFetchRetries    = 3
	numiaAPRsFetchRetryDelay = time.Second * 1
)

// GetFetchPoolAPRsFromNumiaCb returns a callback to fetch pool APRs from Numia.
// It increments the error counter if the pool APRs fetching fails.
// It returns a callback function that returns the pool APRs on success.
func GetFetchPoolAPRsFromNumiaCb(numiaHTTPClient passthroughdomain.NumiaHTTPClient, logger log.Logger) func() (map[uint64]sqspassthroughdomain.PoolAPR, error) {
	return func() (map[uint64]sqspassthroughdomain.PoolAPR, error) {

		for i := 0; i < numiaAPRsFetchRetries; i++ {

			// Fetch pool APRs from the passthrough grpc client
			poolAPRs, err := numiaHTTPClient.GetPoolAPRsRange()
			if err != nil {
				logger.Error("Failed to fetch pool APRs,", zap.Error(err), zap.Int("retry", i))

				time.Sleep(numiaAPRsFetchRetryDelay)
				continue
			}

			// Convert to map
			poolAPRsMap := make(map[uint64]sqspassthroughdomain.PoolAPR, len(poolAPRs))
			for _, poolAPR := range poolAPRs {
				poolAPRsMap[poolAPR.PoolID] = poolAPR
			}

			return poolAPRsMap, nil
		}

		// Increment the error counter
		domain.SQSPassthroughNumiaAPRsFetchErrorCounter.Inc()

		return nil, fmt.Errorf("failed to fetch pool APRs after %d retries", numiaAPRsFetchRetries)
	}
}

// GetFetchPoolPoolFeesFromTimeseries returns a callback to fetch pool fees from timeseries data stack.
// It increments the error counter if the pool fees fetching fails.
// It returns a callback function that returns the pool fees on success.
func GetFetchPoolPoolFeesFromTimeseries(timeseriesHTTPClient passthroughdomain.TimeSeriesHTTPClient, logger log.Logger) func() (map[uint64]sqspassthroughdomain.PoolFee, error) {
	return func() (map[uint64]sqspassthroughdomain.PoolFee, error) {
		// Fetch pool APRs from the passthrough grpc client
		poolFees, err := timeseriesHTTPClient.GetPoolFees()
		if err != nil {
			logger.Error("Failed to fetch pool fees", zap.Error(err))

			// Increment the error counter
			domain.SQSPassthroughTimeseriesPoolFeesFetchErrorCounter.Inc()

			return nil, err
		}

		poolFeesMap := make(map[uint64]sqspassthroughdomain.PoolFee, len(poolFees.Data))
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

		return poolFeesMap, nil
	}
}
