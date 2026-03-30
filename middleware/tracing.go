package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/shiliu-ai/go-atlas/atlas/log"
	"github.com/shiliu-ai/go-atlas/tracing"
)

const HeaderTraceID = "X-Trace-ID"

// Tracing injects OpenTelemetry trace ID into the log context and response header.
// Should be used after otelgin middleware to pick up the active span.
func Tracing() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := tracing.TraceIDFromContext(c.Request.Context())
		if traceID != "" {
			ctx := log.WithTraceID(c.Request.Context(), traceID)
			c.Request = c.Request.WithContext(ctx)
			c.Header(HeaderTraceID, traceID)
		}
		c.Next()
	}
}
