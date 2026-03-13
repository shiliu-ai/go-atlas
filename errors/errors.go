package errors

import (
	"errors"
	"fmt"
)

// Re-export standard errors functions for convenience.
var (
	Is     = errors.Is
	As     = errors.As
	Unwrap = errors.Unwrap
)

// Code represents a business error code.
type Code int

const (
	CodeOK              Code = 0
	CodeBadRequest      Code = 400
	CodeUnauthorized    Code = 401
	CodeForbidden       Code = 403
	CodeNotFound        Code = 404
	CodeConflict        Code = 409
	CodeTooManyRequests Code = 429
	CodeInternal        Code = 500
	CodeBadGateway      Code = 502
	CodeServiceUnavail  Code = 503
)

// Predefined sentinel errors for common cases.
var (
	ErrBadRequest      = New(CodeBadRequest, "bad request")
	ErrUnauthorized    = New(CodeUnauthorized, "unauthorized")
	ErrForbidden       = New(CodeForbidden, "forbidden")
	ErrNotFound        = New(CodeNotFound, "not found")
	ErrConflict        = New(CodeConflict, "conflict")
	ErrTooManyRequests = New(CodeTooManyRequests, "too many requests")
	ErrInternal        = New(CodeInternal, "internal error")
	ErrBadGateway      = New(CodeBadGateway, "bad gateway")
	ErrServiceUnavail  = New(CodeServiceUnavail, "service unavailable")
)

// Error is a structured error with code and message.
type Error struct {
	code       Code
	httpStatus int
	message    string
	msgKey     string
	msgArgs    []any
	cause      error
}

// New creates an error with a business code and a plain message.
func New(code Code, message string) *Error {
	return &Error{code: code, message: message}
}

// NewT creates an error with a business code and an i18n message key.
// The message key is resolved to a localized string in the response layer.
func NewT(code Code, msgKey string, args ...any) *Error {
	return &Error{code: code, msgKey: msgKey, msgArgs: args}
}

// Wrap creates an error wrapping a cause, with a business code and a plain message.
func Wrap(code Code, message string, err error) *Error {
	return &Error{code: code, message: message, cause: err}
}

// WrapT creates an error wrapping a cause, with a business code and an i18n message key.
func WrapT(code Code, msgKey string, err error, args ...any) *Error {
	return &Error{code: code, msgKey: msgKey, msgArgs: args, cause: err}
}

func (e *Error) Error() string {
	msg := e.message
	if msg == "" {
		msg = "i18n:" + e.msgKey
	}
	if e.cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.code, msg, e.cause)
	}
	return fmt.Sprintf("[%d] %s", e.code, msg)
}

func (e *Error) Code() Code       { return e.code }
func (e *Error) Message() string   { return e.message }
func (e *Error) MsgKey() string    { return e.msgKey }
func (e *Error) MsgArgs() []any    { return e.msgArgs }
func (e *Error) HTTPStatus() int   { return e.httpStatus }
func (e *Error) Unwrap() error     { return e.cause }

// Is reports whether target matches this error by business code.
// This allows errors.Is(New(CodeNotFound, "user not found"), ErrNotFound) to return true.
func (e *Error) Is(target error) bool {
	var t *Error
	if As(target, &t) {
		return e.code == t.code
	}
	return false
}

// WithHTTPStatus sets an explicit HTTP status code, overriding the default
// derivation from the business code.
func (e *Error) WithHTTPStatus(status int) *Error {
	clone := *e
	clone.httpStatus = status
	return &clone
}

// WithMessage returns a clone with the given plain message.
func (e *Error) WithMessage(msg string) *Error {
	clone := *e
	clone.message = msg
	clone.msgKey = ""
	clone.msgArgs = nil
	return &clone
}

// WithMsgKey returns a clone with the given i18n message key and optional args.
func (e *Error) WithMsgKey(key string, args ...any) *Error {
	clone := *e
	clone.message = ""
	clone.msgKey = key
	clone.msgArgs = args
	return &clone
}

// FromError extracts *Error from an error. Returns nil if not an *Error.
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if As(err, &e) {
		return e
	}
	return nil
}
