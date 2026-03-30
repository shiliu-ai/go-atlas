package log

import (
	"context"
	"sync"
)

type contextKey int

const (
	traceIDKey   contextKey = iota
	requestIDKey
	fieldsKey
)

// WithTraceID stores trace ID in context.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// TraceIDFromContext extracts trace ID from context.
func TraceIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(traceIDKey).(string)
	return v
}

// WithRequestID stores request ID in context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext extracts request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// WithContextFields attaches extra log fields to the context.
// These fields are automatically included in every log call that receives this context.
func WithContextFields(ctx context.Context, fields ...Field) context.Context {
	existing := ContextFields(ctx)
	merged := make([]Field, len(existing), len(existing)+len(fields))
	copy(merged, existing)
	merged = append(merged, fields...)
	return context.WithValue(ctx, fieldsKey, merged)
}

// ContextFields extracts log fields attached to the context.
func ContextFields(ctx context.Context) []Field {
	v, _ := ctx.Value(fieldsKey).([]Field)
	return v
}

// ContextExtractor extracts Fields from context.
// Register custom extractors to automatically pull values from context into every log entry.
type ContextExtractor func(ctx context.Context) []Field

var (
	extractorsMu sync.RWMutex
	extractors   []ContextExtractor
)

func init() {
	// Built-in extractors for trace_id and request_id.
	RegisterExtractor(func(ctx context.Context) []Field {
		var fields []Field
		if v := TraceIDFromContext(ctx); v != "" {
			fields = append(fields, F("trace_id", v))
		}
		if v := RequestIDFromContext(ctx); v != "" {
			fields = append(fields, F("request_id", v))
		}
		return fields
	})
	// Built-in extractor for context-attached fields.
	RegisterExtractor(func(ctx context.Context) []Field {
		return ContextFields(ctx)
	})
}

// RegisterExtractor adds a custom context extractor.
// Extractors are called in registration order on every log call.
func RegisterExtractor(e ContextExtractor) {
	extractorsMu.Lock()
	defer extractorsMu.Unlock()
	extractors = append(extractors, e)
}

// ExtractFromContext runs all registered extractors and returns the combined fields.
func ExtractFromContext(ctx context.Context) []Field {
	extractorsMu.RLock()
	defer extractorsMu.RUnlock()

	var fields []Field
	for _, e := range extractors {
		fields = append(fields, e(ctx)...)
	}
	return fields
}
