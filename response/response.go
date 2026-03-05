package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shiliu-ai/go-atlas/errors"
	"github.com/shiliu-ai/go-atlas/log"
	"github.com/shiliu-ai/go-atlas/tracing"
)

// R is the unified API response structure.
type R struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}

func newR(c *gin.Context, code int, message string, data any) R {
	return R{
		Code:    code,
		Message: message,
		Data:    data,
		TraceID: tracing.TraceIDFromContext(c.Request.Context()),
	}
}

// OK sends a success response.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, newR(c, 0, "ok", data))
}

// Err sends an error response derived from an error.
func Err(c *gin.Context, err error) {
	if e := errors.FromError(err); e != nil {
		c.JSON(codeToHTTPStatus(e.Code()), newR(c, int(e.Code()), e.Message(), nil))
		return
	}
	log.Error(c.Request.Context(), "unhandled error", log.F("error", err))
	c.JSON(http.StatusInternalServerError, newR(c, int(errors.CodeInternal), "internal error", nil))
}

// Fail sends an error response with a business error code and message.
func Fail(c *gin.Context, code errors.Code, message string) {
	c.JSON(codeToHTTPStatus(code), newR(c, int(code), message, nil))
}

func codeToHTTPStatus(code errors.Code) int {
	if code >= 400 && code < 600 {
		return int(code)
	}
	return http.StatusInternalServerError
}
