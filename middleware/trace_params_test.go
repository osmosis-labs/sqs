package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
	"unsafe"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/middleware"
)

const (
	baseKey      = "base"
	tokenInKey   = "tokenIn"
	tokenInValue = "1000uosmo"
)

// serveTraced runs a GET request with the given raw query through TraceWithParamsMiddleware,
// with a span from tp started ahead of it in place of the echo OTEL middleware.
func serveTraced(t *testing.T, tp *sdktrace.TracerProvider, rawQuery string) {
	t.Helper()
	tracer := tp.Tracer("test")

	e := deliveryhttp.NewEcho()
	m := middleware.InitMiddleware(&domain.CORSConfig{}, &domain.FlightRecordConfig{}, &log.NoOpLogger{})
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx, span := tracer.Start(c.Request().Context(), "request")
			defer span.End()
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	e.Use(m.TraceWithParamsMiddleware())
	e.GET("/router/quote", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/router/quote?"+rawQuery, nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestTraceWithParamsMiddleware(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = tp.Shutdown(t.Context()) }()

	longBase := strings.Repeat("a", 400)
	serveTraced(t, tp, tokenInKey+"="+tokenInValue+"&tokenIn=2uion&"+baseKey+"="+longBase+"&secret=topsecret&randomKey=abc")

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	attrs := make(map[string]string, len(spans[0].Attributes))
	for _, kv := range spans[0].Attributes {
		if kv.Value.Type() == attribute.STRING {
			attrs[string(kv.Key)] = kv.Value.AsString()
		}
	}

	require.Equal(t, map[string]string{
		tokenInKey: tokenInValue,
		baseKey:    strings.Repeat("a", middleware.MaxSpanQueryParamValLen),
	}, attrs)
}

// capturingClient is an otlptrace.Client that marshals every upload to the OTLP wire format
// instead of sending it, recording the marshal errors and the decoded payloads.
type capturingClient struct {
	mu        sync.Mutex
	errs      []error
	resources []*tracepb.ResourceSpans
}

func (*capturingClient) Start(context.Context) error { return nil }
func (*capturingClient) Stop(context.Context) error  { return nil }

func (c *capturingClient) UploadTraces(_ context.Context, protoSpans []*tracepb.ResourceSpans) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rs := range protoSpans {
		wire, err := proto.Marshal(rs)
		if err != nil {
			c.errs = append(c.errs, err)
			continue
		}
		decoded := &tracepb.ResourceSpans{}
		if err := proto.Unmarshal(wire, decoded); err != nil {
			c.errs = append(c.errs, err)
			continue
		}
		c.resources = append(c.resources, decoded)
	}
	return nil
}

// serializedAttributes returns the string attributes of the single exported span, as decoded
// from the OTLP wire format.
func (c *capturingClient) serializedAttributes(t *testing.T) map[string]string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.Empty(t, c.errs)
	require.Len(t, c.resources, 1)
	require.Len(t, c.resources[0].ScopeSpans, 1)
	require.Len(t, c.resources[0].ScopeSpans[0].Spans, 1)

	span := c.resources[0].ScopeSpans[0].Spans[0]
	attrs := make(map[string]string, len(span.Attributes))
	for _, kv := range span.Attributes {
		attrs[kv.Key] = kv.Value.GetStringValue()
	}
	return attrs
}

// TestTraceWithParamsMiddlewareOTLPSerialization checks that request-controlled values always
// survive OTLP protobuf serialization, which rejects invalid UTF-8 and would otherwise drop the
// whole span batch.
func TestTraceWithParamsMiddlewareOTLPSerialization(t *testing.T) {
	maxLen := middleware.MaxSpanQueryParamValLen
	testCases := []struct {
		name     string
		rawQuery string
		key      string
		expected string
	}{
		{
			name:     "multibyte rune straddling the byte limit is dropped whole",
			rawQuery: "filter[search]=" + strings.Repeat("a", maxLen-1) + "%C3%A9%C3%A9",
			key:      "filter[search]",
			expected: strings.Repeat("a", maxLen-1),
		},
		{
			name:     "multibyte rune ending exactly at the byte limit is kept",
			rawQuery: "filter[search]=" + strings.Repeat("a", maxLen-2) + "%C3%A9%C3%A9",
			key:      "filter[search]",
			expected: strings.Repeat("a", maxLen-2) + "é",
		},
		{
			name:     "raw invalid byte is replaced",
			rawQuery: tokenInKey + "=%ff",
			key:      tokenInKey,
			expected: "�",
		},
		{
			name:     "replacement expansion stays within the byte limit",
			rawQuery: tokenInKey + "=" + strings.Repeat("a%ff", maxLen),
			key:      tokenInKey,
			expected: strings.Repeat("a�", maxLen/4),
		},
		{
			name:     "exactly limit-length ASCII is unchanged",
			rawQuery: baseKey + "=" + strings.Repeat("b", maxLen),
			key:      baseKey,
			expected: strings.Repeat("b", maxLen),
		},
		{
			name:     "over-limit ASCII is truncated",
			rawQuery: baseKey + "=" + strings.Repeat("b", maxLen+1),
			key:      baseKey,
			expected: strings.Repeat("b", maxLen),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &capturingClient{}
			exporter, err := otlptrace.New(t.Context(), client)
			require.NoError(t, err)
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			defer func() { _ = tp.Shutdown(t.Context()) }()

			serveTraced(t, tp, tc.rawQuery)

			got := client.serializedAttributes(t)[tc.key]
			require.Equal(t, tc.expected, got)
			require.True(t, utf8.ValidString(got))
			require.LessOrEqual(t, len(got), maxLen)
		})
	}
}

// TestOTLPMarshalRejectsInvalidUTF8 guards the premise of the serialization test above: if
// protobuf stopped rejecting invalid UTF-8, that test would no longer catch a regression.
func TestOTLPMarshalRejectsInvalidUTF8(t *testing.T) {
	kv := &commonpb.KeyValue{
		Key:   tokenInKey,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "\xff"}},
	}
	_, err := proto.Marshal(kv)
	require.Error(t, err)
}

func TestSpanAttributeValue(t *testing.T) {
	maxLen := middleware.MaxSpanQueryParamValLen
	testCases := []struct {
		name     string
		in       string
		expected string
	}{
		{name: "empty", in: "", expected: ""},
		{name: "short ASCII", in: tokenInValue, expected: tokenInValue},
		{
			name:     "four-byte rune straddling the limit",
			in:       strings.Repeat("a", maxLen-3) + "\U0001F600",
			expected: strings.Repeat("a", maxLen-3),
		},
		{
			name:     "valid U+FFFD in input is kept",
			in:       "�",
			expected: "�",
		},
		{
			name:     "truncated multibyte sequence is replaced",
			in:       "a\xe2\x82",
			expected: "a��",
		},
		{
			name:     "only invalid bytes",
			in:       strings.Repeat("\xff", 2*maxLen),
			expected: strings.Repeat("�", maxLen/3),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := middleware.SpanAttributeValue(tc.in)
			require.Equal(t, tc.expected, got)
			require.True(t, utf8.ValidString(got))
			require.LessOrEqual(t, len(got), maxLen)
		})
	}
}

// TestSpanAttributeValueCopies checks that the returned value never shares memory with the
// request-derived input, so a buffered span cannot keep the full request value alive.
func TestSpanAttributeValueCopies(t *testing.T) {
	in := strings.Repeat("a", 4*middleware.MaxSpanQueryParamValLen)
	for _, src := range []string{in[:10], in} {
		got := middleware.SpanAttributeValue(src)
		require.NotSame(t, unsafe.StringData(src), unsafe.StringData(got))
	}
}

func TestAllowedSpanQueryParamsUnique(t *testing.T) {
	seen := make(map[attribute.Key]struct{}, len(middleware.AllowedSpanQueryParams))
	for _, key := range middleware.AllowedSpanQueryParams {
		_, dup := seen[key]
		require.False(t, dup, "duplicate allowlisted query param %q", key)
		seen[key] = struct{}{}
	}
}

// TestAllowedSpanQueryParamsCoverSwagger fails when a query parameter documented in the
// generated swagger spec is missing from the span allowlist. The spec does not document every
// parameter, so this is a floor, not a full equivalence check.
func TestAllowedSpanQueryParamsCoverSwagger(t *testing.T) {
	raw, err := os.ReadFile("../docs/swagger.json")
	require.NoError(t, err)

	var spec struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `json:"name"`
				In   string `json:"in"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(raw, &spec))

	allowed := make(map[string]struct{}, len(middleware.AllowedSpanQueryParams))
	for _, key := range middleware.AllowedSpanQueryParams {
		allowed[string(key)] = struct{}{}
	}

	documented := 0
	for path, ops := range spec.Paths {
		for method, op := range ops {
			for _, param := range op.Parameters {
				if param.In != "query" {
					continue
				}
				documented++
				require.Contains(t, allowed, param.Name, "%s %s query param %q missing from span allowlist", method, path, param.Name)
			}
		}
	}
	require.Positive(t, documented, "swagger spec documents no query params; the guard is vacuous")
}
