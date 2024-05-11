package middleware

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// GoMiddleware represent the data-struct for middleware
type GoMiddleware struct {
	corsConfig domain.CORSConfig
}

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
func InitMiddleware(corsConfig *domain.CORSConfig) *GoMiddleware {
	return &GoMiddleware{
		corsConfig: *corsConfig,
	}
}

// InstrumentMiddleware will handle the instrumentation middleware
func (m *GoMiddleware) InstrumentMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
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

		duration := time.Since(start).Seconds()

		// Observe the duration with the histogram
		requestLatency.WithLabelValues(requestMethod, requestPath).Observe(duration)

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
