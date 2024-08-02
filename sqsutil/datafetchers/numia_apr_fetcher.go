package datafetchers

import (
	"github.com/osmosis-labs/sqs/domain"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// GetFetchtPoolAPRsFromNumiaCb returns a callback to fetch pool APRs from Numia.
// It increments the error counter if the pool APRs fetching fails.
// It returns a callback function that returns the pool APRs on success.
func GetFetchtPoolAPRsFromNumiaCb(numiaHTTPClient passthroughdomain.NumiaHTTPClient, logger log.Logger) func() []passthroughdomain.PoolAPR {
	return func() []passthroughdomain.PoolAPR {
		// Fetch pool APRs from the passthrough grpc client
		poolAPRs, err := numiaHTTPClient.GetPoolAPRsRange()
		if err != nil {
			logger.Error("Failed to fetch pool APRs", zap.Error(err))

			// Increment the error counter
			domain.SQSPassthroughNumiaAPRsFetchErrorCounter.WithLabelValues().Inc()
		}
		return poolAPRs
	}
}
