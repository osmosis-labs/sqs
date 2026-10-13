package http

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/sqs/validator"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// DefaultClient represents default HTTP client for issuing outgoing HTTP requests.
var DefaultClient = &http.Client{
	Timeout:   5 * time.Second, // Adjusted timeout to 5 seconds
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

type shutdownSignalKey struct{}

// WithShutdownSignal installs a BaseContext on server that carries a channel closed when
// server.Shutdown is called. Long-lived handlers such as SSE streams select on it via
// ShutdownSignal so they end and let graceful shutdown complete. Ordinary requests keep
// their context and are allowed to drain.
func WithShutdownSignal(server *http.Server) {
	shutdown := make(chan struct{})
	// Shutdown runs registered callbacks on every call, so guard against a double close.
	var once sync.Once
	server.RegisterOnShutdown(func() { once.Do(func() { close(shutdown) }) })

	baseCtx := context.WithValue(context.Background(), shutdownSignalKey{}, (<-chan struct{})(shutdown))
	server.BaseContext = func(net.Listener) context.Context { return baseCtx }
}

// ShutdownSignal returns the channel installed by WithShutdownSignal, or nil (blocks forever)
// if the request was not served by such a server.
func ShutdownSignal(ctx context.Context) <-chan struct{} {
	ch, _ := ctx.Value(shutdownSignalKey{}).(<-chan struct{})
	return ch
}

// RequestUnmarshaler is any type capable to unmarshal data from HTTP request to itself.
type RequestUnmarshaler interface {
	UnmarshalHTTPRequest(c echo.Context) error
}

// UnmarshalRequest unmarshals HTTP request into m.
func UnmarshalRequest(c echo.Context, m RequestUnmarshaler) error {
	return m.UnmarshalHTTPRequest(c)
}

// ParseRequest encapsulates the request unmarshalling and validation logic.
// It unmarshals the request and validates it if the request implements the Validator interface.
func ParseRequest(c echo.Context, req RequestUnmarshaler) error {
	if err := UnmarshalRequest(c, req); err != nil {
		return err
	}

	v, ok := req.(validator.Validator)
	if !ok {
		return nil
	}
	return validator.Validate(v)
}
