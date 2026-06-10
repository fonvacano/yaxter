// Package log provides the structured JSON logger (ARCHITECTURE.md §5):
// zerolog, with trace_id / span_id / request_id enrichment from context.
// Never log tokens or PII bodies.
package log

import (
	"context"
	"io"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}

// New returns a JSON logger writing to w. Unknown levels fall back to info.
func New(w io.Writer, level, service string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(level)
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	return zerolog.New(w).Level(lvl).With().
		Timestamp().
		Str("service", service).
		Logger()
}

// WithRequestID stores a request ID in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request ID from the context, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// Ctx returns l enriched with trace_id/span_id (from the OTel span context)
// and request_id, when present in ctx. Returns a pointer so pointer-receiver
// methods (zerolog v1.35+) are callable on the return value directly.
func Ctx(ctx context.Context, l zerolog.Logger) *zerolog.Logger {
	lc := l.With()
	if id := RequestID(ctx); id != "" {
		lc = lc.Str("request_id", id)
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		lc = lc.Str("trace_id", sc.TraceID().String()).
			Str("span_id", sc.SpanID().String())
	}
	out := lc.Logger()
	return &out
}
