package domain

import "github.com/prometheus/client_golang/prometheus"

var (
	// sqs_ingest_usecase_process_block_duration
	//
	// histogram that measures the duration of processing a block in milliseconds in ingest usecase
	//
	// Has the following labels:
	// * height - the height of the block being processed
	SQSIngestUsecaseProcessBlockDurationMetricName = "sqs_ingest_usecase_process_block_duration"

	// sqs_ingest_usecase_process_block_error
	//
	// counter that measures the number of errors that occur during processing a block in ingest usecase
	//
	// Has the following labels:
	// * err - the error message occurred
	// * height - the height of the block being processed
	SQSIngestUsecaseProcessBlockErrorMetricName = "sqs_ingest_usecase_process_block_error_total"

	// sqs_ingest_usecase_parse_pool_error_total
	//
	// counter that measures the number of errors that occur during pool parsing in ingest usecase
	//
	// Has the following labels:
	// * err - the error message occurred
	SQSIngestUsecaseParsePoolErrorMetricName = "sqs_ingest_usecase_parse_pool_error_total"

	// sqs_pricing_worker_compute_error_counter
	//
	// counter that measures the number of errors that occur during pricing worker computation
	//
	// Has the following labels:
	// * height - the height of the block being processed
	SQSPricingWorkerComputeErrorCounterMetricName = "sqs_pricing_worker_compute_error_total"

	// sqs_pricing_worker_compute_duration
	//
	// gauge that tracks duration of pricing worker computation
	//
	// Has the following labels:
	// * height - the height of the block being processed
	// * num_pools - the number of pools being processed
	SQSPricingWorkerComputeDurationMetricName = "sqs_pricing_worker_compute_duration"

	SQSIngestHandlerProcessBlockDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: SQSIngestUsecaseProcessBlockDurationMetricName,
			Help: "histogram that measures the duration of processing a block in milliseconds in ingest usecase",
		},
		[]string{"height"},
	)

	SQSIngestHandlerProcessBlockErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SQSIngestUsecaseProcessBlockErrorMetricName,
			Help: "counter that measures the number of errors that occur during processing a block in ingest usecase",
		},
		[]string{"err", "height"},
	)

	SQSIngestHandlerPoolParseErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SQSIngestUsecaseParsePoolErrorMetricName,
			Help: "counter that measures the number of errors that occur during pool parsing in ingest usecase",
		},
		[]string{"err"},
	)

	SQSPricingWorkerComputeErrorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: SQSPricingWorkerComputeErrorCounterMetricName,
			Help: "counter that measures the number of errors that occur during pricing worker computation",
		},
		[]string{"height"},
	)

	SQSPricingWorkerComputeDurationGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: SQSPricingWorkerComputeDurationMetricName,
			Help: "gauge that tracks duration of pricing worker computation",
		},
		[]string{"height", "num_pools"},
	)
)

func init() {
	prometheus.MustRegister(SQSIngestHandlerProcessBlockDurationHistogram)
	prometheus.MustRegister(SQSIngestHandlerProcessBlockErrorCounter)
	prometheus.MustRegister(SQSIngestHandlerPoolParseErrorCounter)
	prometheus.MustRegister(SQSPricingWorkerComputeDurationGauge)
	prometheus.MustRegister(SQSPricingWorkerComputeErrorCounter)
}
