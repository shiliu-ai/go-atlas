package log

import "context"

// Level represents logging severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger is the standard logging interface. Implementations can wrap zap, logrus, slog, etc.
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)
	WithFields(fields ...Field) Logger
}

// Field is a key-value pair for structured logging.
type Field struct {
	Key   string
	Value any
}

// F is a shorthand for creating a Field.
func F(key string, value any) Field {
	return Field{Key: key, Value: value}
}
