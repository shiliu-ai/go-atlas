package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// fakeLock is a test Locker recording calls.
type fakeLock struct {
	acquire    bool
	acquireErr error
	acquired   int
	released   int
}

func (f *fakeLock) Acquire(context.Context) (bool, error) {
	f.acquired++
	return f.acquire, f.acquireErr
}

func (f *fakeLock) Release(context.Context) error {
	f.released++
	return nil
}

func testManager() *Manager {
	return &Manager{cron: cron.New(), logger: log.NewDefault(log.LevelError)}
}

func TestManager_Register_Validation(t *testing.T) {
	m := testManager()
	noop := func(context.Context) error { return nil }

	if err := m.Register(Job{Spec: "@every 1m", Run: noop}); err == nil {
		t.Error("expected error for missing Name")
	}
	if err := m.Register(Job{Name: "x", Run: noop}); err == nil {
		t.Error("expected error for missing Spec")
	}
	if err := m.Register(Job{Name: "x", Spec: "@every 1m"}); err == nil {
		t.Error("expected error for missing Run")
	}
	if err := m.Register(Job{Name: "x", Spec: "not-a-cron", Run: noop}); err == nil {
		t.Error("expected error for invalid spec")
	}
	if err := m.Register(Job{Name: "ok", Spec: "@every 1m", Run: noop}); err != nil {
		t.Errorf("valid job: %v", err)
	}
}

func TestManager_RunJob_LockGatesExecution(t *testing.T) {
	cases := []struct {
		name        string
		lock        *fakeLock
		wantRuns    int
		wantRelease int
	}{
		{"no lock runs", nil, 1, 0},
		{"lock granted runs and releases", &fakeLock{acquire: true}, 1, 1},
		{"lock denied skips", &fakeLock{acquire: false}, 0, 0},
		{"lock error skips", &fakeLock{acquireErr: errors.New("boom")}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runs := 0
			job := Job{Name: "t", Spec: "@every 1h", Run: func(context.Context) error { runs++; return nil }}
			if tc.lock != nil {
				job.Lock = tc.lock
			}

			testManager().runJob(job)

			if runs != tc.wantRuns {
				t.Errorf("runs = %d, want %d", runs, tc.wantRuns)
			}
			if tc.lock != nil && tc.lock.released != tc.wantRelease {
				t.Errorf("released = %d, want %d", tc.lock.released, tc.wantRelease)
			}
		})
	}
}

func TestManager_RunJob_RecoversPanic(t *testing.T) {
	job := Job{Name: "panicky", Spec: "@every 1h", Run: func(context.Context) error { panic("boom") }}
	// Must not panic.
	testManager().runJob(job)
}

func TestManager_StartStop_RunsJob(t *testing.T) {
	m := testManager()
	done := make(chan struct{}, 1)
	err := m.Register(Job{Name: "tick", Spec: "@every 100ms", Run: func(context.Context) error {
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	}})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = m.Stop(context.Background()) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run within 2s")
	}
}

func TestManager_RunJob_UsesCancellableContext(t *testing.T) {
	m := testManager()
	m.ctx, m.cancel = context.WithCancel(context.Background())

	started := make(chan struct{})
	done := make(chan struct{})
	job := Job{Name: "long", Spec: "@every 1h", Run: func(ctx context.Context) error {
		close(started)
		<-ctx.Done() // block until the manager context is cancelled
		return ctx.Err()
	}}

	go func() {
		m.runJob(job)
		close(done)
	}()

	<-started
	m.cancel() // simulate Stop signalling in-flight jobs

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runJob did not observe context cancellation")
	}
}

func TestManager_Stop_CancelsContext(t *testing.T) {
	m := testManager()
	m.ctx, m.cancel = context.WithCancel(context.Background())

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if m.ctx.Err() == nil {
		t.Error("Stop did not cancel the manager context")
	}
}

func newTestReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
	return reader
}

func sumCounter(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range sum.DataPoints {
					total += dp.Value
				}
			}
		}
	}
	return total
}

func TestScheduler_Metrics(t *testing.T) {
	reader := newTestReader(t)
	m := testManager() // existing helper: &Manager{cron, logger}

	m.runJob(Job{Name: "ok", Spec: "@every 1h", Run: func(context.Context) error { return nil }})
	m.runJob(Job{Name: "boom", Spec: "@every 1h", Run: func(context.Context) error { return errors.New("x") }})

	if got := sumCounter(t, reader, "scheduler.job.runs"); got != 2 {
		t.Fatalf("scheduler.job.runs = %d, want 2", got)
	}
}
