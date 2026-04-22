package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// SpanFromContext returns the span currently bound to ctx.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceIDFromContext returns the active trace ID as a string, or "" if none.
func TraceIDFromContext(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}
