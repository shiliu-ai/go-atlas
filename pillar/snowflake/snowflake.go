// Package snowflake is an atlas Pillar that assigns each instance a unique
// Snowflake worker ID (0..1023) — automatically via a Redis lease by default,
// or from a static config override. If the Redis lease cannot be renewed and
// the worker ID may have expired, generation is stopped (fail-safe) so two
// instances never share a worker ID and produce duplicate IDs.
package snowflake

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/artifact/id"
)

// ErrUnavailable is returned by Generate when the worker ID lease has been lost
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
	// WorkerID, if set, statically pins the worker ID and disables Redis
	// allocation. Leave unset for automatic allocation.
	WorkerID *int64 `mapstructure:"worker_id"`
	// Redis is the backend for automatic allocation (required unless WorkerID
	// is set).
	Redis RedisConfig `mapstructure:"redis"`
	// TTL is the worker-ID lease lifetime. Must exceed RenewInterval +
	// SafetyMargin. Default 30s.
	TTL time.Duration `mapstructure:"ttl"`
	// RenewInterval is how often the lease is renewed. Default 10s.
	RenewInterval time.Duration `mapstructure:"renew_interval"`
	// SafetyMargin closes the gate this long before the lease would expire.
	// Default 5s.
	SafetyMargin time.Duration `mapstructure:"safety_margin"`
	// FailMode is "safe" (default: stop generating on lease loss) or
	// "besteffort" (keep generating; risks duplicate IDs).
	FailMode string `mapstructure:"fail_mode"`
	// KeyPrefix namespaces the Redis lease keys. Default "snowflake:worker:".
	KeyPrefix string `mapstructure:"key_prefix"`
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
	if c.TTL <= c.RenewInterval+c.SafetyMargin {
		return fmt.Errorf("snowflake: ttl (%s) must exceed renew_interval + safety_margin (%s)",
			c.TTL, c.RenewInterval+c.SafetyMargin)
	}
	return nil
}

// Generator wraps an id.Snowflake with an atomic gate so generation can be
// stopped when the worker-ID lease is lost.
type Generator struct {
	sf   *id.Snowflake
	open atomic.Bool
}

func (g *Generator) setOpen(v bool) { g.open.Store(v) }

// Generate returns a new unique snowflake ID, or ErrUnavailable if the gate is
// closed (lease lost).
func (g *Generator) Generate() (int64, error) {
	if !g.open.Load() {
		return 0, ErrUnavailable
	}
	return g.sf.Generate()
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

	// leaseExpires is the local estimate of when the current lease ends; only
	// touched by the single renewal goroutine (and Init before it starts).
	leaseExpires time.Time

	cancel context.CancelFunc
}

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

// renewOnce renews the lease and updates the gate. On success the gate opens
// and the lease is extended; on failure the gate closes once now+SafetyMargin
// reaches the lease expiry (fail-safe), unless besteffort mode is set.
func (m *Manager) renewOnce(ctx context.Context, now time.Time) {
	ok, err := m.allocator.Renew(ctx)
	if err == nil && ok {
		m.leaseExpires = now.Add(m.ttl)
		m.gen.setOpen(true)
		return
	}
	if err != nil {
		m.logger.Warn(ctx, "snowflake lease renew failed", log.F("error", err))
	} else {
		m.logger.Warn(ctx, "snowflake lease lost")
	}
	if m.failSafe && !now.Add(m.safety).Before(m.leaseExpires) {
		if m.gen.open.Swap(false) {
			m.logger.Error(ctx, "snowflake gate closed: lease unrenewed within safety margin")
		}
	}
}
