package telemetry

import "github.com/prometheus/client_golang/prometheus"

var (
	// sqs_orderbook_usecase_get_active_orders_error_total
	//
	// counter that measures the number of errors that occur during getting active orders in orderbook usecase
	//
	// Has the following labels:
	// * contract - the address of the orderbook contract
	// * address - address of the user wallet
	// * err - the error message occurred
	GetActiveOrdersErrorMetricName = "sqs_orderbook_usecase_get_active_orders_error_total"

	// sqs_orderbook_usecase_get_tick_by_id_not_found_total
	//
	// counter that measures the number of times a tick is not found by id in orderbook usecase
	GetTickByIDNotFoundMetricName = "sqs_orderbook_usecase_get_tick_by_id_not_found_total"

	// sqs_orderbook_usecase_create_limit_order_error_total
	//
	// counter that measures the number of errors that occur during creating limit order in orderbook
	//
	// Has the following labels:
	// * order - the order from orderbook that was attempted to be created as a limit order
	// * err - the error message occurred
	CreateLimitOrderErrorMetricName = "sqs_orderbook_usecase_create_limit_order_error_total"

	GetActiveOrdersErrorCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: GetActiveOrdersErrorMetricName,
			Help: "counter that measures the number of errors that occur during retrieving active orders from orderbook contract",
		},
	)

	GetTickByIDNotFoundCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: GetTickByIDNotFoundMetricName,
			Help: "counter that measures the number of not found ticks by ID that occur during retrieving active orders from orderbook contract",
		},
	)

	CreateLimitOrderErrorCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: CreateLimitOrderErrorMetricName,
			Help: "counter that measures the number errors that occur during creating a limit order orderbook from orderbook order",
		},
	)
)

func init() {
	prometheus.MustRegister(GetActiveOrdersErrorCounter)
	prometheus.MustRegister(GetTickByIDNotFoundCounter)
	prometheus.MustRegister(CreateLimitOrderErrorCounter)
}
