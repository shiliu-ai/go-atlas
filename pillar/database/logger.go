package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm/logger"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// gormLogger adapts atlas log.Logger to GORM's logger.Interface,
// so that SQL logs share the same output format, level control,
// and carry context fields (trace_id, request_id, etc.).
type gormLogger struct {
	logger                    log.Logger
	level                     logger.LogLevel
	slowThreshold             time.Duration
	ignoreRecordNotFoundError bool
}

// newGormLogger creates a GORM logger backed by an atlas logger. When
// ignoreRecordNotFoundError is true, gorm.ErrRecordNotFound is treated as a
// normal (non-error) result and not logged at ERROR — mirroring GORM's own
// logger.Config.IgnoreRecordNotFoundError. This keeps First()+errors.Is control
// flow (e.g. "user has no active row") from flooding the error stream.
func newGormLogger(l log.Logger, level logger.LogLevel, slowThreshold time.Duration, ignoreRecordNotFoundError bool) logger.Interface {
	return &gormLogger{
		logger:                    l,
		level:                     level,
		slowThreshold:             slowThreshold,
		ignoreRecordNotFoundError: ignoreRecordNotFoundError,
	}
}

func (g *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	return &gormLogger{
		logger:                    g.logger,
		level:                     level,
		slowThreshold:             g.slowThreshold,
		ignoreRecordNotFoundError: g.ignoreRecordNotFoundError,
	}
}

func (g *gormLogger) Info(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= logger.Info {
		g.logger.Info(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormLogger) Warn(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= logger.Warn {
		g.logger.Warn(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormLogger) Error(ctx context.Context, msg string, args ...interface{}) {
	if g.level >= logger.Error {
		g.logger.Error(ctx, fmt.Sprintf(msg, args...))
	}
}

func (g *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if g.level <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []log.Field{
		log.F("latency_ms", float64(elapsed)/float64(time.Millisecond)),
		log.F("rows", rows),
		log.F("sql", sql),
	}

	switch {
	case err != nil && g.level >= logger.Error &&
		!(g.ignoreRecordNotFoundError && errors.Is(err, logger.ErrRecordNotFound)):
		g.logger.Error(ctx, err.Error(), fields...)
	case g.slowThreshold > 0 && elapsed > g.slowThreshold && g.level >= logger.Warn:
		g.logger.Warn(ctx, "slow sql", fields...)
	case g.level >= logger.Info:
		g.logger.Info(ctx, "sql", fields...)
	}
}

// Ensure gormLogger implements GORM's logger.Interface.
var _ logger.Interface = (*gormLogger)(nil)
