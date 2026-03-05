package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shiliu-ai/go-atlas/log"
)

// Logging logs each HTTP request with duration, status, and method.
func Logging(logger log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if raw != "" {
			path = path + "?" + raw
		}

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []log.Field{
			log.F("status", status),
			log.F("method", c.Request.Method),
			log.F("path", path),
			log.F("latency", latency.String()),
			log.F("ip", c.ClientIP()),
		}

		if len(c.Errors) > 0 {
			logger.Error(c.Request.Context(), c.Errors.String(), fields...)
		} else if status >= 500 {
			logger.Error(c.Request.Context(), "server error", fields...)
		} else if status >= 400 {
			logger.Warn(c.Request.Context(), "client error", fields...)
		} else {
			logger.Info(c.Request.Context(), "request", fields...)
		}
	}
}
