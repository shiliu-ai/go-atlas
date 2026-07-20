package atlas

import (
	"context"
	"testing"
	"time"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// TestShutdown_PreShutdownDelay verifies shutdown() waits the configured
// pre_shutdown_delay (after flipping to draining) before returning.
func TestShutdown_PreShutdownDelay(t *testing.T) {
	a := &Atlas{
		srv:      newServer(serverConfig{PreShutdownDelay: 80 * time.Millisecond}),
		registry: newPillarRegistry(),
		logger:   log.NewDefault(log.LevelError),
	}

	start := time.Now()
	if err := a.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 80*time.Millisecond {
		t.Fatalf("shutdown returned after %v, want >= 80ms (pre_shutdown_delay)", elapsed)
	}
	if a.readinessValue() != readinessDraining {
		t.Errorf("readiness = %v, want draining", a.readinessValue())
	}
}

// TestShutdown_NoDelayByDefault verifies shutdown() does not wait when
// pre_shutdown_delay is unset (default 0).
func TestShutdown_NoDelayByDefault(t *testing.T) {
	a := &Atlas{
		srv:      newServer(serverConfig{}),
		registry: newPillarRegistry(),
		logger:   log.NewDefault(log.LevelError),
	}

	start := time.Now()
	if err := a.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("shutdown took %v with no delay configured, want fast", elapsed)
	}
}
