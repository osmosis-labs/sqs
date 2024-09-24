package http

import (
	"context"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/trace"
)

// Span returns the current span from the context.
func Span(c echo.Context) (context.Context, trace.Span) {
	ctx := c.Request().Context()
	span := trace.SpanFromContext(ctx)
	return ctx, span
}

// RecordSpanError records an error ( if any ) for the span.
func RecordSpanError(ctx context.Context, span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
	}
	// Note: we do not end the span here as it is ended in the middleware.
}
