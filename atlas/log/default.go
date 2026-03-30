package log

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// Option configures the default logger.
type Option func(*slogConfig)

type slogConfig struct {
	out    io.Writer
	json   bool
}

// WithOutput sets the log output writer.
func WithOutput(w io.Writer) Option {
	return func(c *slogConfig) { c.out = w }
}

// WithJSON enables JSON output format.
func WithJSON() Option {
	return func(c *slogConfig) { c.json = true }
}

// slogLogger implements Logger backed by log/slog.
type slogLogger struct {
	logger *slog.Logger
}

func toSlogLevel(l Level) slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewDefault creates a default logger backed by slog, writing to stdout.
func NewDefault(level Level, opts ...Option) Logger {
	cfg := &slogConfig{out: os.Stdout}
	for _, opt := range opts {
		opt(cfg)
	}

	handlerOpts := &slog.HandlerOptions{Level: toSlogLevel(level)}

	var handler slog.Handler
	if cfg.json {
		handler = slog.NewJSONHandler(cfg.out, handlerOpts)
	} else {
		handler = slog.NewTextHandler(cfg.out, handlerOpts)
	}

	return &slogLogger{logger: slog.New(handler)}
}

func (l *slogLogger) Debug(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, slog.LevelDebug, msg, fields)
}

func (l *slogLogger) Info(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, slog.LevelInfo, msg, fields)
}

func (l *slogLogger) Warn(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, slog.LevelWarn, msg, fields)
}

func (l *slogLogger) Error(ctx context.Context, msg string, fields ...Field) {
	l.log(ctx, slog.LevelError, msg, fields)
}

func (l *slogLogger) WithFields(fields ...Field) Logger {
	attrs := fieldsToAttrs(fields)
	return &slogLogger{
		logger: l.logger.With(attrsToArgs(attrs)...),
	}
}

func (l *slogLogger) log(ctx context.Context, level slog.Level, msg string, fields []Field) {
	if !l.logger.Enabled(ctx, level) {
		return
	}

	// Collect context fields + call-site fields.
	ctxFields := ExtractFromContext(ctx)
	allFields := make([]Field, 0, len(ctxFields)+len(fields))
	allFields = append(allFields, ctxFields...)
	allFields = append(allFields, fields...)

	attrs := fieldsToAttrs(allFields)
	l.logger.LogAttrs(ctx, level, msg, attrs...)
}

func fieldsToAttrs(fields []Field) []slog.Attr {
	attrs := make([]slog.Attr, len(fields))
	for i, f := range fields {
		attrs[i] = slog.Any(f.Key, f.Value)
	}
	return attrs
}

func attrsToArgs(attrs []slog.Attr) []any {
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	return args
}
