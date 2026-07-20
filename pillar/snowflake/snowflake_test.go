package snowflake

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/artifact/id"
)

func testGen(t *testing.T, workerID int64) *Generator {
	t.Helper()
	sf, err := id.NewSnowflake(workerID)
	if err != nil {
		t.Fatalf("new snowflake: %v", err)
	}
	g := &Generator{}
	g.setSnowflake(sf)
	g.setOpen(true)
	return g
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	m := &Manager{
		gen:      testGen(t, 1),
		logger:   log.NewDefault(log.LevelError),
		failSafe: true,
		ttl:      30 * time.Second,
		renew:    10 * time.Second,
		safety:   5 * time.Second,
	}
	m.setLease(time.Now().Add(m.ttl))
	return m
}

func TestGenerator_Gate(t *testing.T) {
	g := testGen(t, 1)
	if _, err := g.Generate(); err != nil {
		t.Fatalf("open gate: %v", err)
	}
	g.setOpen(false)
	if _, err := g.Generate(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("closed gate err = %v, want ErrUnavailable", err)
	}
}

func TestManager_CheckLease_FailSafe(t *testing.T) {
	t.Run("lease valid keeps gate open", func(t *testing.T) {
		m := testManager(t)
		m.setLease(time.Now().Add(time.Hour))
		m.checkLease(time.Now())
		if !m.gen.open.Load() {
			t.Fatal("gate should stay open while lease is valid")
		}
	})
	t.Run("within safety margin closes gate", func(t *testing.T) {
		m := testManager(t)
		m.setLease(time.Now().Add(-time.Second)) // effectively expired
		m.checkLease(time.Now())
		if m.gen.open.Load() {
			t.Fatal("gate should close within safety margin (fail-safe)")
		}
	})
	t.Run("besteffort never closes", func(t *testing.T) {
		m := testManager(t)
		m.failSafe = false
		m.setLease(time.Now().Add(-time.Hour))
		m.checkLease(time.Now())
		if !m.gen.open.Load() {
			t.Fatal("besteffort should keep the gate open")
		}
	})
}

// reAllocator tracks Acquire/Renew/Release/Close and drives outcomes.
type reAllocator struct {
	mu       sync.Mutex
	renewOK  bool
	renewErr error
	nextID   int64
	acquires int
	releases int
	closes   int
}

func (a *reAllocator) Acquire(context.Context) (int64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acquires++
	return a.nextID, nil
}
func (a *reAllocator) Renew(context.Context) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.renewOK, a.renewErr
}
func (a *reAllocator) Release(context.Context) error {
	a.mu.Lock()
	a.releases++
	a.mu.Unlock()
	return nil
}
func (a *reAllocator) Close() error { a.mu.Lock(); a.closes++; a.mu.Unlock(); return nil }

func TestManager_TryRenew_Success(t *testing.T) {
	m := testManager(t)
	m.allocator = &reAllocator{renewOK: true}
	m.gen.setOpen(false)
	m.setLease(time.Now().Add(-time.Hour))

	m.tryRenew(context.Background())

	if !m.gen.open.Load() {
		t.Fatal("successful renew should open the gate")
	}
	if time.Until(m.lease()) < 20*time.Second {
		t.Fatal("successful renew should extend the lease by ttl")
	}
}

func TestManager_TryRenew_ReacquiresOnLeaseLoss(t *testing.T) {
	m := testManager(t)
	// renewOK=false, no error => lease lost => must re-acquire a fresh id.
	a := &reAllocator{renewOK: false, nextID: 42}
	m.allocator = a
	m.gen.setOpen(false)

	m.tryRenew(context.Background())

	a.mu.Lock()
	got := a.acquires
	a.mu.Unlock()
	if got != 1 {
		t.Fatalf("expected 1 re-acquire on lease loss, got %d", got)
	}
	if !m.gen.open.Load() {
		t.Fatal("gate should reopen after successful re-acquire")
	}
	// generator should now produce IDs from the new worker id (42).
	if _, err := m.gen.Generate(); err != nil {
		t.Fatalf("generate after reacquire: %v", err)
	}
}

func TestManager_StartStop_ReleasesAndCloses(t *testing.T) {
	m := testManager(t)
	a := &reAllocator{renewOK: true}
	m.allocator = a

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.releases != 1 {
		t.Errorf("releases = %d, want 1", a.releases)
	}
	if a.closes != 1 {
		t.Errorf("closes = %d, want 1", a.closes)
	}
}

func TestConfig_Validate(t *testing.T) {
	base := Config{TTL: 30 * time.Second, RenewInterval: 10 * time.Second, SafetyMargin: 5 * time.Second}
	if err := base.validate(); err == nil {
		t.Error("auto mode requires redis.addr")
	}
	wid := int64(3)
	static := base
	static.WorkerID = &wid
	if err := static.validate(); err != nil {
		t.Errorf("static valid: %v", err)
	}
	bad := static
	bad.TTL = 12 * time.Second
	if err := bad.validate(); err == nil {
		t.Error("ttl must exceed renew+safety")
	}
	neg := static
	neg.SafetyMargin = -time.Second
	if err := neg.validate(); err == nil {
		t.Error("negative durations must be rejected")
	}
	oor := base
	big := int64(2000)
	oor.WorkerID = &big
	if err := oor.validate(); err == nil {
		t.Error("worker_id out of [0,1023] must be rejected")
	}
}

func TestStaticAllocator(t *testing.T) {
	a := &staticAllocator{workerID: 7}
	if got, err := a.Acquire(context.Background()); err != nil || got != 7 {
		t.Fatalf("Acquire = (%d,%v)", got, err)
	}
	if ok, err := a.Renew(context.Background()); !ok || err != nil {
		t.Fatalf("Renew = (%v,%v)", ok, err)
	}
	if err := a.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewRedisAllocator_Unreachable(t *testing.T) {
	cfg := Config{Redis: RedisConfig{Addr: "127.0.0.1:1"}, TTL: 30 * time.Second, KeyPrefix: "snowflake:worker:"}
	if _, err := newRedisAllocator(cfg); err == nil {
		t.Fatal("expected error for unreachable redis")
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

func TestSnowflake_Metrics(t *testing.T) {
	reader := newTestReader(t)
	m := testManager(t) // existing helper
	m.allocator = &reAllocator{renewOK: true}

	m.tryRenew(context.Background()) // -> renewed
	m.allocator = &reAllocator{renewOK: false, nextID: 9}
	m.tryRenew(context.Background()) // -> lost + reacquired
	m.setLease(time.Now().Add(-time.Second))
	m.checkLease(time.Now()) // -> gate_closed

	if got := sumCounter(t, reader, "snowflake.lease.events"); got < 3 {
		t.Fatalf("snowflake.lease.events = %d, want >= 3", got)
	}
}
