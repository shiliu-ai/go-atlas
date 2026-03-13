package serviceclient

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// contextKey is an unexported type used for context values.
type contextKey int

const headersKey contextKey = iota

// headersToForward lists the headers that are automatically propagated
// from incoming requests to outgoing service calls.
var headersToForward = []string{
	"Authorization",
	"X-Request-ID",
	"X-Trace-ID",
}

// ForwardHeaders is a Gin middleware that captures specified headers from
// the incoming request and stores them in context for automatic forwarding
// to downstream service calls.
//
// Usage:
//
//	router.Use(serviceclient.ForwardHeaders())
//
// This middleware should be placed after authentication middleware so that
// the Authorization header is present. It is automatically applied when
// using atlas's default middleware stack with services configured.
func ForwardHeaders(extra ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		headers := make(http.Header)
		for _, key := range headersToForward {
			if v := c.GetHeader(key); v != "" {
				headers.Set(key, v)
			}
		}
		for _, key := range extra {
			if v := c.GetHeader(key); v != "" {
				headers.Set(key, v)
			}
		}
		ctx := context.WithValue(c.Request.Context(), headersKey, headers)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// WithHeaders returns a new context carrying the given headers for forwarding.
// This is useful for programmatic service calls outside of Gin handlers
// (e.g. cron jobs, message consumers).
func WithHeaders(ctx context.Context, headers http.Header) context.Context {
	return context.WithValue(ctx, headersKey, headers)
}

// forwardHeaders copies stored headers from context into the outgoing request.
// Uses Add instead of Set to preserve multi-value headers.
func forwardHeaders(ctx context.Context, req *http.Request) {
	stored, ok := ctx.Value(headersKey).(http.Header)
	if !ok {
		return
	}
	for key, vals := range stored {
		for _, v := range vals {
			req.Header.Add(key, v)
		}
	}
}
