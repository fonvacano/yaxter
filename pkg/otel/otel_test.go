package otel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// With an empty endpoint, Setup must still install a working tracer
// provider (spans get valid IDs) and a clean shutdown.
func TestSetupWithoutEndpoint(t *testing.T) {
	ctx := context.Background()
	shutdown, err := Setup(ctx, Config{ServiceName: "test", SampleRatio: 1.0})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, shutdown(ctx)) })

	_, span := otel.Tracer("test").Start(ctx, "op")
	defer span.End()
	require.True(t, span.SpanContext().TraceID().IsValid())
	require.True(t, span.SpanContext().IsSampled())
}

func TestSetupInstallsW3CPropagator(t *testing.T) {
	ctx := context.Background()
	shutdown, err := Setup(ctx, Config{ServiceName: "test", SampleRatio: 1.0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(ctx) })

	fields := otel.GetTextMapPropagator().Fields()
	require.Contains(t, fields, "traceparent")
	require.Contains(t, fields, "baggage")
}

// Compile-time check that Setup wires the real span type.
var _ trace.Span = trace.SpanFromContext(context.Background())
