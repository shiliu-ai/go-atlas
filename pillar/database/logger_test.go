package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm/logger"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// captureLogger is a log.Logger test double that records which level was used.
type captureLogger struct {
	errCount  int
	warnCount int
	infoCount int
	lastMsg   string
}

func (c *captureLogger) Debug(ctx context.Context, msg string, fields ...log.Field) {}
func (c *captureLogger) Info(ctx context.Context, msg string, fields ...log.Field) {
	c.infoCount++
	c.lastMsg = msg
}
func (c *captureLogger) Warn(ctx context.Context, msg string, fields ...log.Field) {
	c.warnCount++
	c.lastMsg = msg
}
func (c *captureLogger) Error(ctx context.Context, msg string, fields ...log.Field) {
	c.errCount++
	c.lastMsg = msg
}
func (c *captureLogger) WithFields(fields ...log.Field) log.Logger { return c }

func traceOnce(g logger.Interface, err error) {
	g.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT 1", 0
	}, err)
}

// When IgnoreRecordNotFoundError is enabled (the default), a record-not-found
// result is normal control flow (First() + errors.Is) and must NOT be logged as
// ERROR — otherwise every subscription-less user floods the error stream.
func TestGormLogger_IgnoresRecordNotFound(t *testing.T) {
	cap := &captureLogger{}
	g := newGormLogger(cap, logger.Error, 0, true)

	traceOnce(g, logger.ErrRecordNotFound)

	if cap.errCount != 0 {
		t.Fatalf("record-not-found should not log ERROR when ignored, got errCount=%d", cap.errCount)
	}
}

// A genuine error must still be logged at ERROR even when record-not-found is ignored.
func TestGormLogger_RealErrorStillLogged(t *testing.T) {
	cap := &captureLogger{}
	g := newGormLogger(cap, logger.Error, 0, true)

	traceOnce(g, errors.New("connection refused"))

	if cap.errCount != 1 {
		t.Fatalf("real error must log ERROR, got errCount=%d", cap.errCount)
	}
}

// When explicitly opted out (ignore=false), record-not-found is logged as ERROR,
// matching GORM's own IgnoreRecordNotFoundError=false behavior.
func TestGormLogger_RecordNotFoundLoggedWhenNotIgnored(t *testing.T) {
	cap := &captureLogger{}
	g := newGormLogger(cap, logger.Error, 0, false)

	traceOnce(g, logger.ErrRecordNotFound)

	if cap.errCount != 1 {
		t.Fatalf("record-not-found should log ERROR when not ignored, got errCount=%d", cap.errCount)
	}
}
