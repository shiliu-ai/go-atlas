package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shiliu-ai/go-atlas/atlas/errors"
	"github.com/shiliu-ai/go-atlas/atlas/i18n"
	"github.com/shiliu-ai/go-atlas/atlas/log"
	"github.com/shiliu-ai/go-atlas/tracing"
)

// R is the unified API response structure.
type R struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
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
	msg := i18n.T(c.Request.Context(), i18n.MsgOK)
	c.JSON(http.StatusOK, newR(c, 0, msg, data))
}

// Err sends an error response derived from an error.
func Err(c *gin.Context, err error) {
	if e := errors.FromError(err); e != nil {
		msg := e.Message()
		if key := e.MsgKey(); key != "" {
			msg = i18n.T(c.Request.Context(), key, e.MsgArgs()...)
		}
		status := e.HTTPStatus()
		if status == 0 {
			status = codeToHTTPStatus(e.Code())
		}
		if status >= 500 {
			log.Error(c.Request.Context(), "server error", log.F("code", e.Code()), log.F("error", err))
		} else if status >= 400 {
			log.Warn(c.Request.Context(), "client error", log.F("code", e.Code()), log.F("error", err))
		}
		c.JSON(status, newR(c, int(e.Code()), msg, nil))
		return
	}
	log.Error(c.Request.Context(), "unhandled error", log.F("error", err))
	msg := i18n.T(c.Request.Context(), i18n.MsgInternalError)
	c.JSON(http.StatusInternalServerError, newR(c, int(errors.CodeInternal), msg, nil))
}

// AbortErr sends an error response and aborts the Gin handler chain.
// Use this in middleware (e.g. authentication) where subsequent handlers must not run.
func AbortErr(c *gin.Context, err error) {
	Err(c, err)
	c.Abort()
}

// Fail sends an error response with a business error code and message.
func Fail(c *gin.Context, code errors.Code, message string) {
	Err(c, errors.New(code, message))
}

// FailT sends an error response with a translated message key.
// The key is looked up in the i18n bundle using the request's locale.
func FailT(c *gin.Context, code errors.Code, key string, args ...any) {
	Err(c, errors.NewT(code, key, args...))
}

var codeHTTPStatus = map[errors.Code]int{
	errors.CodeBadRequest:      http.StatusBadRequest,
	errors.CodeUnauthorized:    http.StatusUnauthorized,
	errors.CodeForbidden:       http.StatusForbidden,
	errors.CodeNotFound:        http.StatusNotFound,
	errors.CodeConflict:        http.StatusConflict,
	errors.CodeTooManyRequests: http.StatusTooManyRequests,
	errors.CodeInternal:        http.StatusInternalServerError,
	errors.CodeBadGateway:      http.StatusBadGateway,
	errors.CodeServiceUnavail:  http.StatusServiceUnavailable,
}

func codeToHTTPStatus(code errors.Code) int {
	if status, ok := codeHTTPStatus[code]; ok {
		return status
	}
	// Unmapped codes in the standard HTTP error range are used directly as HTTP status.
	if c := int(code); c >= 400 && c <= 599 {
		return c
	}
	// Custom business codes (e.g. 100001) return HTTP 200;
	// the business error is communicated via the "code" field in the response body.
	return http.StatusOK
}
