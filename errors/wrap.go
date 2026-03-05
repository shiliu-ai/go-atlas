package errors

import "errors"

// Re-export standard errors functions for convenience.
var (
	Is     = errors.Is
	As     = errors.As
	Unwrap = errors.Unwrap
)
