package log

import (
	"context"
	"sync/atomic"
)

var global atomic.Pointer[Logger]

// SetGlobal sets the global logger. Safe for concurrent use.
func SetGlobal(l Logger) {
	global.Store(&l)
}

// Global returns the global logger. If not set, returns a default logger.
func Global() Logger {
	if p := global.Load(); p != nil {
		return *p
	}
	// Lazy init: create default and attempt to store it.
	// If another goroutine already stored one, use that instead.
	l := Logger(NewDefault(LevelInfo))
	if global.CompareAndSwap(nil, &l) {
		return l
	}
	return *global.Load()
}

// Convenience functions using the global logger.

func Debug(ctx context.Context, msg string, fields ...Field) { Global().Debug(ctx, msg, fields...) }
func Info(ctx context.Context, msg string, fields ...Field)  { Global().Info(ctx, msg, fields...) }
func Warn(ctx context.Context, msg string, fields ...Field)  { Global().Warn(ctx, msg, fields...) }
func Error(ctx context.Context, msg string, fields ...Field) { Global().Error(ctx, msg, fields...) }
