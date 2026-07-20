package snowflake

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/artifact/id"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	sf, err := id.NewSnowflake(1)
	if err != nil {
		t.Fatalf("new snowflake: %v", err)
	}
	g := &Generator{sf: sf}
	g.setOpen(true)
	return &Manager{
		gen:      g,
		logger:   log.NewDefault(log.LevelError),
		failSafe: true,
		ttl:      30 * time.Second,
		safety:   5 * time.Second,
	}
}

func TestGenerator_Gate(t *testing.T) {
	m := testManager(t)

	if _, err := m.Generate(); err != nil {
		t.Fatalf("gate open: Generate returned error: %v", err)
	}

	m.gen.setOpen(false)
	if _, err := m.Generate(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("gate closed: err = %v, want ErrUnavailable", err)
	}
}

// fakeAllocator drives Renew outcomes for renewOnce tests.
type fakeAllocator struct {
	renewOK  bool
	renewErr error
}

func (f fakeAllocator) Acquire(context.Context) (int64, error) { return 1, nil }
func (f fakeAllocator) Renew(context.Context) (bool, error)    { return f.renewOK, f.renewErr }
func (f fakeAllocator) Release(context.Context) error          { return nil }

func TestManager_RenewOnce_FailSafe(t *testing.T) {
	t.Run("successful renew keeps gate open and extends lease", func(t *testing.T) {
		m := testManager(t)
		m.allocator = fakeAllocator{renewOK: true}
		m.leaseExpires = time.Now().Add(-time.Hour) // even if stale, success re-opens
		m.gen.setOpen(false)

		m.renewOnce(context.Background(), time.Now())

		if !m.gen.open.Load() {
			t.Fatal("gate should be open after successful renew")
		}
		if time.Until(m.leaseExpires) < 20*time.Second {
			t.Fatal("lease should be extended by ttl on success")
		}
	})

	t.Run("failed renew within safety margin closes gate", func(t *testing.T) {
		m := testManager(t)
		m.allocator = fakeAllocator{renewOK: false}
		m.leaseExpires = time.Now().Add(-time.Second) // lease effectively gone

		m.renewOnce(context.Background(), time.Now())

		if m.gen.open.Load() {
			t.Fatal("gate should be closed (fail-safe) when lease is past the safety margin")
		}
	})

	t.Run("failed renew with lease time remaining keeps gate open", func(t *testing.T) {
		m := testManager(t)
		m.allocator = fakeAllocator{renewErr: errors.New("transient")}
		m.leaseExpires = time.Now().Add(time.Hour) // plenty of lease left

		m.renewOnce(context.Background(), time.Now())

		if !m.gen.open.Load() {
			t.Fatal("a single transient failure with lease left should not close the gate")
		}
	})

	t.Run("besteffort never closes the gate", func(t *testing.T) {
		m := testManager(t)
		m.failSafe = false
		m.allocator = fakeAllocator{renewOK: false}
		m.leaseExpires = time.Now().Add(-time.Hour)

		m.renewOnce(context.Background(), time.Now())

		if !m.gen.open.Load() {
			t.Fatal("besteffort mode should keep generating even after lease loss")
		}
	})
}

func TestConfig_Validate(t *testing.T) {
	base := Config{TTL: 30 * time.Second, RenewInterval: 10 * time.Second, SafetyMargin: 5 * time.Second}

	// redis mode without addr -> error
	if err := base.validate(); err == nil {
		t.Error("expected error: auto mode requires redis.addr")
	}

	// static mode is valid without redis addr
	wid := int64(3)
	static := base
	static.WorkerID = &wid
	if err := static.validate(); err != nil {
		t.Errorf("static config should be valid: %v", err)
	}

	// ttl must exceed renew_interval + safety_margin
	bad := static
	bad.TTL = 12 * time.Second // 12 <= 10 + 5
	if err := bad.validate(); err == nil {
		t.Error("expected error: ttl must exceed renew_interval + safety_margin")
	}
}

func TestStaticAllocator(t *testing.T) {
	a := &staticAllocator{workerID: 7}
	got, err := a.Acquire(context.Background())
	if err != nil || got != 7 {
		t.Fatalf("Acquire = (%d, %v), want (7, nil)", got, err)
	}
	if ok, err := a.Renew(context.Background()); !ok || err != nil {
		t.Fatalf("static Renew = (%v, %v), want (true, nil)", ok, err)
	}
	if err := a.Release(context.Background()); err != nil {
		t.Fatalf("static Release: %v", err)
	}
}

func TestNewRedisAllocator_Unreachable(t *testing.T) {
	cfg := Config{Redis: RedisConfig{Addr: "127.0.0.1:1"}, TTL: 30 * time.Second, KeyPrefix: "snowflake:worker:"}
	if _, err := newRedisAllocator(cfg); err == nil {
		t.Fatal("expected error constructing allocator against unreachable redis")
	}
}
