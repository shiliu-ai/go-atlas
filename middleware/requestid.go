package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shiliu-ai/go-atlas/log"
)

const HeaderRequestID = "X-Request-ID"

// RequestID injects a unique request ID into context and response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(HeaderRequestID)
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Header(HeaderRequestID, requestID)

		ctx := log.WithRequestID(c.Request.Context(), requestID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
