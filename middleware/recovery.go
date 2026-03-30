package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/shiliu-ai/go-atlas/atlas/log"
)

// Recovery recovers from panics and logs the stack trace.
func Recovery(logger log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())
				logger.Error(c.Request.Context(), "panic recovered",
					log.F("error", r),
					log.F("stack", stack),
				)
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
