package middleware

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"

	"time"

	"go.uber.org/zap"
	gotrace "golang.org/x/exp/trace"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// GoMiddleware represent the data-struct for middleware
type GoMiddleware struct {
	corsConfig domain.CORSConfig
	logger     log.Logger
}

const (
	// Name of the flight recorder trace
	flightRecorderTraceName = "flight-trace.out"
	// The latency threshold in milliseconds to record traces
	flightRecorderThresholdMs = 500
)

var (
	// total number of requests counter
	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_requests_total",
			Help: "Total number of requests.",
		},
		[]string{"method", "endpoint"},
	)

	// request latency histogram
	requestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sqs_request_duration_seconds",
			Help:    "Histogram of request latencies.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// flight recorder
	recordFlightOnce sync.Once
)

func init() {
	prometheus.MustRegister(requestsTotal)
	prometheus.MustRegister(requestLatency)
}

// CORS will handle the CORS middleware
func (m *GoMiddleware) CORS(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set("Access-Control-Allow-Origin", m.corsConfig.AllowedOrigin)
		c.Response().Header().Set("Access-Control-Allow-Headers", m.corsConfig.AllowedHeaders)
		c.Response().Header().Set("Access-Control-Allow-Methods", m.corsConfig.AllowedMethods)
		return next(c)
	}
}

// InitMiddleware initialize the middleware
func InitMiddleware(corsConfig *domain.CORSConfig, logger log.Logger) *GoMiddleware {
	return &GoMiddleware{
		corsConfig: *corsConfig,
		logger:     logger,
	}
}

// InstrumentMiddleware will handle the instrumentation middleware
func (m *GoMiddleware) InstrumentMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	// Set up the flight recorder.
	fr := gotrace.NewFlightRecorder()
	err := fr.Start()
	if err != nil {
		m.logger.Error("failed to start flight recorder", zap.Error(err))
	}

	return func(c echo.Context) error {
		start := time.Now()

		requestMethod := c.Request().Method
		requestPath, err := domain.ParseURLPath(c)
		if err != nil {
			return err
		}

		// Increment the request counter
		requestsTotal.WithLabelValues(requestMethod, requestPath).Inc()

		// Insert the request path into the context
		ctx := c.Request().Context()
		ctx = context.WithValue(ctx, domain.RequestPathCtxKey, requestPath)
		request := c.Request().WithContext(ctx)
		c.SetRequest(request)

		err = next(c)

		duration := time.Since(start)

		// Observe the duration with the histogram
		requestLatency.WithLabelValues(requestMethod, requestPath).Observe(duration.Seconds())

		// Record outliers to the flight recorder for further analysis
		if duration > flightRecorderThresholdMs*time.Millisecond {
			recordFlightOnce.Do(func() {
				// Note: we skip error handling since we don't want to interrupt the request handling
				// with tracing errors.

				// Grab the snapshot.
				var b bytes.Buffer
				_, err = fr.WriteTo(&b)
				if err != nil {
					m.logger.Error("failed to write trace to buffer", zap.Error(err))
					return
				}

				// Write it to a file.
				err = os.WriteFile(flightRecorderTraceName, b.Bytes(), 0o755)
				if err != nil {
					m.logger.Error("failed to write trace to file", zap.Error(err))
					return
				}

				err = fr.Stop()
				if err != nil {
					fmt.Println("failed to stop flight recorder: ", err)
					m.logger.Error("failed to stop fligt recorder", zap.Error(err))
					return
				}
			})
		}

		return err
	}
}

// Middleware to create a span and capture request parameters
func (m *GoMiddleware) TraceWithParamsMiddleware(tracerName string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tracer := otel.Tracer(tracerName)

			// Extract the existing span context from the incoming request
			parentCtx := otel.GetTextMapPropagator().Extract(c.Request().Context(), propagation.HeaderCarrier(c.Request().Header))

			// Start a new span representing the request
			// The span ends when the request is complete
			ctx, span := tracer.Start(parentCtx, c.Path(), trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()

			span.SetAttributes(attribute.String("http.method", c.Request().Method))

			// Inject the span context back into the Echo context and request context
			c.SetRequest(c.Request().WithContext(ctx))

			// Iterate through query parameters and add them as attributes to the span
			// Ensure to filter out any sensitive parameters here
			for key, values := range c.QueryParams() {
				// As a simple approach, we're adding only the first value of each parameter
				// Consider handling multiple values differently if necessary
				span.SetAttributes(attribute.String(key, values[0]))
			}

			// Proceed with the request handling
			return next(c)
		}
	}
}
