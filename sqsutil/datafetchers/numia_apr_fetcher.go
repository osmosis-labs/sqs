package datafetchers

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	sqspassthroughdomain "github.com/osmosis-labs/osmosis/v28/ingest/types/passthroughdomain"
	"github.com/osmosis-labs/sqs/domain"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

const (
	// numiaAPRsFetchRetries is the number of retries to fetch pool APRs from Numia.
	numiaAPRsFetchRetries = 3
	// numiaAPRsFetchRetryDelay is the delay between retries to fetch pool APRs from Numia.
	numiaAPRsFetchRetryDelay = time.Second * 1

	// poolFeesFetchRetries is the number of retries to fetch pool fees from the timeseries API.
	poolFeesFetchRetries = 3
	// poolFeesFetchRetryDelay is the delay between retries to fetch pool fees from the timeseries API.
	poolFeesFetchRetryDelay = time.Second * 1
)

// GetFetchPoolAPRsFromNumiaCb returns a callback to fetch pool APRs from Numia.
// It tries up to numiaAPRsFetchRetries times, waiting numiaAPRsFetchRetryDelay between attempts.
// A response with no pools counts as a failed attempt, since caching it would erase the APR data of every pool.
// It increments the error counter only if every attempt fails, and then returns the last error.
// It returns a callback function that returns the pool APRs on success.
func GetFetchPoolAPRsFromNumiaCb(numiaHTTPClient passthroughdomain.NumiaHTTPClient, logger log.Logger) func() (map[uint64]sqspassthroughdomain.PoolAPR, error) {
	return func() (map[uint64]sqspassthroughdomain.PoolAPR, error) {
		var err error
		for attempt := range numiaAPRsFetchRetries {
			if attempt > 0 {
				time.Sleep(numiaAPRsFetchRetryDelay)
			}

			// Fetch pool APRs from the passthrough grpc client
			var poolAPRs []sqspassthroughdomain.PoolAPR
			poolAPRs, err = numiaHTTPClient.GetPoolAPRsRange()
			if err != nil {
				logger.Warn("Failed to fetch pool APRs", zap.Error(err), zap.Int("attempt", attempt+1))
				continue
			}

			if len(poolAPRs) == 0 {
				err = errors.New("pool APRs response has no pools")
				logger.Warn("Failed to fetch pool APRs", zap.Error(err), zap.Int("attempt", attempt+1))
				continue
			}

			// Convert to map
			poolAPRsMap := make(map[uint64]sqspassthroughdomain.PoolAPR, len(poolAPRs))
			for _, poolAPR := range poolAPRs {
				poolAPRsMap[poolAPR.PoolID] = poolAPR
			}

			return poolAPRsMap, nil
		}

		logger.Error("Failed to fetch pool APRs after retries", zap.Error(err), zap.Int("attempts", numiaAPRsFetchRetries))

		// Increment the error counter
		domain.SQSPassthroughNumiaAPRsFetchErrorCounter.Inc()

		return nil, fmt.Errorf("fetching pool APRs from Numia failed after %d attempts: %w", numiaAPRsFetchRetries, err)
	}
}

// GetFetchPoolPoolFeesFromTimeseries returns a callback to fetch pool fees from timeseries data stack.
// It tries up to poolFeesFetchRetries times, waiting poolFeesFetchRetryDelay between attempts.
// A response with no pools counts as a failed attempt, since caching it would erase the fee data of every pool.
// It increments the error counter only if every attempt fails, and then returns the last error.
// It returns a callback function that returns the pool fees on success.
func GetFetchPoolPoolFeesFromTimeseries(timeseriesHTTPClient passthroughdomain.TimeSeriesHTTPClient, logger log.Logger) func() (map[uint64]sqspassthroughdomain.PoolFee, error) {
	return func() (map[uint64]sqspassthroughdomain.PoolFee, error) {
		var err error
		for attempt := range poolFeesFetchRetries {
			if attempt > 0 {
				time.Sleep(poolFeesFetchRetryDelay)
			}

			var poolFeesMap map[uint64]sqspassthroughdomain.PoolFee
			poolFeesMap, err = fetchPoolFeesFromTimeseries(timeseriesHTTPClient, logger)
			if err != nil {
				logger.Warn("Failed to fetch pool fees", zap.Error(err), zap.Int("attempt", attempt+1))
				continue
			}

			return poolFeesMap, nil
		}

		logger.Error("Failed to fetch pool fees after retries", zap.Error(err), zap.Int("attempts", poolFeesFetchRetries))

		// Increment the error counter
		domain.SQSPassthroughTimeseriesPoolFeesFetchErrorCounter.Inc()

		return nil, fmt.Errorf("fetching pool fees from timeseries failed after %d attempts: %w", poolFeesFetchRetries, err)
	}
}

// fetchPoolFeesFromTimeseries fetches pool fees once and maps them by pool ID.
// Returns an error if the request fails or the response has no pools.
func fetchPoolFeesFromTimeseries(timeseriesHTTPClient passthroughdomain.TimeSeriesHTTPClient, logger log.Logger) (map[uint64]sqspassthroughdomain.PoolFee, error) {
	poolFees, err := timeseriesHTTPClient.GetPoolFees()
	if err != nil {
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

	if len(poolFeesMap) == 0 {
		return nil, errors.New("pool fees response has no pools")
	}

	return poolFeesMap, nil
}
