package domain

import "github.com/prometheus/client_golang/prometheus"

var (
	// sqs_ingest_usecase_process_block_duration_ms
	//
	// histogram that measures the duration of processing a block in milliseconds in ingest usecase
	//
	// Has the following labels:
	// * height - the height of the block being processed
	SQSIngestUsecaseProcessBlockDurationMetricName = "sqs_ingest_usecase_process_block_duration_ms"

	// sqs_ingest_usecase_process_block_error
	//
	// counter that measures the number of errors that occur during processing a block in ingest usecase
	//
	// Has the following labels:
	// * err - the error message occurred
	// * height - the height of the block being processed
	SQSIngestUsecaseProcessBlockErrorMetricName = "sqs_ingest_usecase_process_block_error"

	// sqs_ingest_usecase_parse_pool_error
	//
	// counter that measures the number of errors that occur during pool parsing in ingest usecase
	//
	// Has the following labels:
	// * err - the error message occurred
	SQSIngestUsecaseParsePoolErrorMetricName = "sqs_ingest_usecase_parse_pool_error"

	SQSIngestHandlerProcessBlockDurationMsHistogram = prometheus.NewHistogramVec(
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
)
