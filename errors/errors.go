package errors

import "fmt"

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

// Error is a structured error with code and message.
type Error struct {
	code    Code
	message string
	cause   error
}

func New(code Code, message string) *Error {
	return &Error{code: code, message: message}
}

func Wrap(code Code, message string, err error) *Error {
	return &Error{code: code, message: message, cause: err}
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.code, e.message, e.cause)
	}
	return fmt.Sprintf("[%d] %s", e.code, e.message)
}

func (e *Error) Code() Code     { return e.code }
func (e *Error) Message() string { return e.message }
func (e *Error) Unwrap() error   { return e.cause }

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
