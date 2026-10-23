package telemetry

import "github.com/prometheus/client_golang/prometheus"

const (
	// Skip reasons for SkippedCounter.
	SkipReasonMalformed    = "malformed"
	SkipReasonPoolType     = "pool_type"
	SkipReasonPoolNotFound = "pool_not_found"

	// Sources for BlockSourceCounter.
	BlockSourceBlockResults = "block_results"
	BlockSourceTxSearch     = "tx_search"
)

var (
	// sqs_trades_ingest_ingested_total
	//
	// counter of trades ingested.
	//
	// Has the following labels:
	// * source - "swap" or "orderbookFill"
	// * direction - "buy", "sell" or "none"
	IngestedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_trades_ingest_ingested_total",
			Help: "counter of trades ingested from token_swapped events",
		},
		[]string{"source", "direction"},
	)

	// sqs_trades_ingest_skipped_total
	//
	// counter of token_swapped events that were not ingested.
	//
	// Has the following labels:
	// * reason - "malformed", "pool_type" or "pool_not_found"
	SkippedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_trades_ingest_skipped_total",
			Help: "counter of token_swapped events skipped during trade ingestion",
		},
		[]string{"reason"},
	)

	// sqs_trades_ingest_pricing_failed_total
	//
	// counter of trades stored without a USD value because the asset could not be priced.
	// The trade itself is still stored.
	//
	// Has the following labels:
	// * source - "swap" or "orderbookFill"
	PricingFailedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_trades_ingest_pricing_failed_total",
			Help: "counter of trades stored with a null USD value because pricing failed",
		},
		[]string{"source"},
	)

	// sqs_trades_ingest_block_fetch_error_total
	//
	// counter of failed attempts to fetch a block or its results from the node RPC.
	BlockFetchErrorCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sqs_trades_ingest_block_fetch_error_total",
			Help: "counter of failed block or block_results fetches during trade ingestion",
		},
	)

	// sqs_trades_ingest_block_source_total
	//
	// counter of blocks whose tx results were read from each source.
	// tx_search is used when the node keeps no block results (discard_abci_responses).
	//
	// Has the following labels:
	// * source - "block_results" or "tx_search"
	BlockSourceCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_trades_ingest_block_source_total",
			Help: "counter of blocks whose tx results trade ingestion read from block_results or the tx index",
		},
		[]string{"source"},
	)

	// sqs_trades_ingest_gap_blocks_total
	//
	// counter of blocks skipped because the ingester fell further behind than it catches up.
	// Trades in these blocks are missing from the store.
	GapBlocksCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sqs_trades_ingest_gap_blocks_total",
			Help: "counter of blocks skipped by trade ingestion, whose trades are missing",
		},
	)

	// sqs_trades_ingest_last_height
	//
	// gauge of the last block height whose trades were ingested.
	LastHeightGauge = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "sqs_trades_ingest_last_height",
			Help: "last block height whose trades were ingested",
		},
	)

	// sqs_trades_ingest_repo_trim_evicted_total
	//
	// counter of trades evicted from memory after the retention period.
	RepoEvictedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "sqs_trades_ingest_repo_trim_evicted_total",
			Help: "counter of trades evicted from the in-memory store after the retention period",
		},
	)

	// sqs_trades_query_list_requests_total
	//
	// Has the following labels:
	// * scope - "pool", "denom" or "pool_denom"
	// * filtered_by_wallet - "true" or "false"
	ListRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_trades_query_list_requests_total",
			Help: "counter of /trades/list requests",
		},
		[]string{"scope", "filtered_by_wallet"},
	)

	// sqs_trades_query_aggregates_requests_total
	//
	// Has the following labels:
	// * scope - "pool", "denom" or "pool_denom"
	// * filtered_by_wallet - "true" or "false"
	AggregatesRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_trades_query_aggregates_requests_total",
			Help: "counter of /trades/aggregates requests",
		},
		[]string{"scope", "filtered_by_wallet"},
	)
)

func init() {
	prometheus.MustRegister(IngestedCounter)
	prometheus.MustRegister(SkippedCounter)
	prometheus.MustRegister(PricingFailedCounter)
	prometheus.MustRegister(BlockFetchErrorCounter)
	prometheus.MustRegister(BlockSourceCounter)
	prometheus.MustRegister(GapBlocksCounter)
	prometheus.MustRegister(LastHeightGauge)
	prometheus.MustRegister(RepoEvictedCounter)
	prometheus.MustRegister(ListRequestsCounter)
	prometheus.MustRegister(AggregatesRequestsCounter)
}
