package log

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func TestNewEmitsJSONWithServiceField(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, "info", "api")
	l.Info().Msg("hello")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, "api", rec["service"])
	require.Equal(t, "hello", rec["message"])
	require.Equal(t, "info", rec["level"])
	require.Contains(t, rec, "time")
}

func TestNewRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, "warn", "api")
	l.Info().Msg("dropped")
	require.Zero(t, buf.Len())
}

func TestCtxAddsTraceAndRequestIDs(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, "info", "api")

	traceID, _ := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := trace.SpanIDFromHex("0123456789abcdef")
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	ctx = WithRequestID(ctx, "req-42")

	Ctx(ctx, l).Info().Msg("traced")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rec))
	require.Equal(t, "0123456789abcdef0123456789abcdef", rec["trace_id"])
	require.Equal(t, "0123456789abcdef", rec["span_id"])
	require.Equal(t, "req-42", rec["request_id"])
}
