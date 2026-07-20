// Package snowflake is an atlas Pillar that assigns each instance a unique
// Snowflake worker ID (0..1023) — automatically via a Redis lease by default,
// or from a static config override.
//
// A background watchdog closes a generation gate before the lease could expire,
// so generation stops (fail-safe) rather than risk two instances sharing a
// worker ID. If the lease is lost, the pillar re-acquires a fresh worker ID and
// resumes. Uniqueness holds as long as the process is not stalled (e.g. a GC or
// scheduling pause) for longer than roughly ttl - renew_interval; size the
// timings accordingly for stall-prone workloads.
package snowflake

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/artifact/id"
)

// ErrUnavailable is returned by Generate when the worker-ID lease has been lost
// and the fail-safe gate is closed.
var ErrUnavailable = errors.New("snowflake: worker ID lease lost, generation unavailable")

// RedisConfig configures the Redis backend used for automatic allocation.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// Config configures the snowflake Pillar.
type Config struct {
	WorkerID      *int64        `mapstructure:"worker_id"`
	Redis         RedisConfig   `mapstructure:"redis"`
	TTL           time.Duration `mapstructure:"ttl"`
	RenewInterval time.Duration `mapstructure:"renew_interval"`
	SafetyMargin  time.Duration `mapstructure:"safety_margin"`
	FailMode      string        `mapstructure:"fail_mode"`
	KeyPrefix     string        `mapstructure:"key_prefix"`
}

func (c Config) withDefaults() Config {
	if c.TTL == 0 {
		c.TTL = 30 * time.Second
	}
	if c.RenewInterval == 0 {
		c.RenewInterval = 10 * time.Second
	}
	if c.SafetyMargin == 0 {
		c.SafetyMargin = 5 * time.Second
	}
	if c.KeyPrefix == "" {
		c.KeyPrefix = "snowflake:worker:"
	}
	return c
}

func (c Config) validate() error {
	if c.WorkerID == nil && c.Redis.Addr == "" {
		return fmt.Errorf("snowflake: redis.addr required for automatic worker id allocation (or set worker_id)")
	}
	if c.WorkerID != nil && (*c.WorkerID < 0 || *c.WorkerID > maxWorkerID) {
		return fmt.Errorf("snowflake: worker_id must be in [0, %d]", maxWorkerID)
	}
	if c.TTL <= 0 || c.RenewInterval <= 0 || c.SafetyMargin < 0 {
		return fmt.Errorf("snowflake: ttl and renew_interval must be > 0 and safety_margin >= 0")
	}
	if c.TTL <= c.RenewInterval+c.SafetyMargin {
		return fmt.Errorf("snowflake: ttl (%s) must exceed renew_interval + safety_margin (%s)",
			c.TTL, c.RenewInterval+c.SafetyMargin)
	}
	return nil
}

// Generator wraps an id.Snowflake with an atomic gate and a swappable generator
// (the worker ID can change if the lease is lost and re-acquired).
type Generator struct {
	sf   atomic.Pointer[id.Snowflake]
	open atomic.Bool
}

func (g *Generator) setOpen(v bool)                { g.open.Store(v) }
func (g *Generator) setSnowflake(sf *id.Snowflake) { g.sf.Store(sf) }

// Generate returns a new unique ID, or ErrUnavailable if the gate is closed.
func (g *Generator) Generate() (int64, error) {
	if !g.open.Load() {
		return 0, ErrUnavailable
	}
	return g.sf.Load().Generate()
}

type snowMetrics struct {
	once   sync.Once
	events metric.Int64Counter
}

func (m *Manager) recordEvent(event string) {
	m.metrics.once.Do(func() {
		meter := otel.Meter("github.com/shiliu-ai/go-atlas/pillar/snowflake")
		m.metrics.events, _ = meter.Int64Counter("snowflake.lease.events",
			metric.WithDescription("Worker-ID lease events (renewed|renew_failed|lost|reacquired|gate_closed)"))
	})
	if m.metrics.events != nil {
		m.metrics.events.Add(context.Background(), 1,
			metric.WithAttributes(attribute.String("event", event)))
	}
}

// Manager is the snowflake Pillar.
type Manager struct {
	logger    log.Logger
	allocator Allocator
	gen       *Generator

	static   bool
	failSafe bool
	ttl      time.Duration
	renew    time.Duration
	safety   time.Duration

	// leaseExpiresNano is the local estimate (unix nano) of when the lease ends;
	// written by the renewer goroutine, read by the watchdog goroutine.
	leaseExpiresNano atomic.Int64

	wg     sync.WaitGroup
	cancel context.CancelFunc

	metrics snowMetrics
}

func (m *Manager) setLease(t time.Time) { m.leaseExpiresNano.Store(t.UnixNano()) }
func (m *Manager) lease() time.Time     { return time.Unix(0, m.leaseExpiresNano.Load()) }

// Generate returns a new unique snowflake ID (see Generator.Generate).
func (m *Manager) Generate() (int64, error) { return m.gen.Generate() }

// MustGenerate returns a new ID or panics.
func (m *Manager) MustGenerate() int64 {
	v, err := m.Generate()
	if err != nil {
		panic(err)
	}
	return v
}

// tryRenew renews the lease with a bounded timeout. On success it extends the
// lease and opens the gate; if the lease is definitively lost (renew reports we
// no longer own it) it re-acquires a fresh worker ID.
func (m *Manager) tryRenew(ctx context.Context) {
	rctx, cancel := context.WithTimeout(ctx, m.renew)
	defer cancel()

	ok, err := m.allocator.Renew(rctx)
	if err == nil && ok {
		m.setLease(time.Now().Add(m.ttl))
		m.gen.setOpen(true)
		m.recordEvent("renewed")
		return
	}
	if err != nil {
		m.logger.Warn(ctx, "snowflake lease renew failed", log.F("error", err))
		m.recordEvent("renew_failed")
		return // transient: watchdog closes the gate if the lease runs out
	}
	// Not the owner anymore: the lease was lost. Re-acquire a fresh worker ID.
	m.logger.Warn(ctx, "snowflake lease lost, re-acquiring worker id")
	m.recordEvent("lost")
	m.reacquire(ctx)
}

// reacquire claims a new worker ID and rebuilds the generator. It is bounded so
// a slow/partitioned Redis cannot stall the renewer indefinitely (the watchdog
// keeps the gate closed meanwhile).
func (m *Manager) reacquire(ctx context.Context) {
	rctx, cancel := context.WithTimeout(ctx, m.renew)
	defer cancel()

	// Release the old lease first (owner-checked no-op if we no longer hold it)
	// so we never keep two worker-id leases at once.
	_ = m.allocator.Release(rctx)

	wid, err := m.allocator.Acquire(rctx)
	if err != nil {
		m.logger.Warn(ctx, "snowflake re-acquire failed", log.F("error", err))
		return
	}
	sf, err := id.NewSnowflake(wid)
	if err != nil {
		m.logger.Error(ctx, "snowflake re-acquire produced invalid worker id", log.F("error", err))
		return
	}
	m.gen.setSnowflake(sf)
	m.setLease(time.Now().Add(m.ttl))
	m.gen.setOpen(true)
	m.recordEvent("reacquired")
	m.logger.Info(ctx, "snowflake worker id re-acquired", log.F("worker_id", wid))
}

// checkLease is the watchdog: it closes the gate once the lease is within the
// safety margin of expiry, on wall-clock, independent of renewal progress.
func (m *Manager) checkLease(now time.Time) {
	if !m.failSafe {
		return
	}
	if now.Add(m.safety).Before(m.lease()) {
		return
	}
	if m.gen.open.Swap(false) {
		m.logger.Error(context.Background(), "snowflake gate closed: lease within safety margin")
		m.recordEvent("gate_closed")
	}
}
