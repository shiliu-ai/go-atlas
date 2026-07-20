package snowflake

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/artifact/id"
	"github.com/shiliu-ai/go-atlas/atlas"
)

// Pillar returns an atlas.Option that registers the snowflake Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		m := &Manager{}
		for _, opt := range opts {
			opt(m)
		}
		a.Register(m)
	}
}

// Of retrieves the snowflake Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager { return atlas.Use[*Manager](a) }

// Option configures the snowflake Pillar.
type Option func(*Manager)

// WithAllocator injects a custom worker-ID Allocator, replacing the default
// config-driven Redis allocation. Use it to back allocation with a different
// coordinator (etcd, Consul, a database, ...) or a fake in tests. When set, a
// Redis address or static worker_id is not required in config.
func WithAllocator(a Allocator) Option {
	return func(m *Manager) { m.allocator = a }
}

// WithRedisClient makes the default Redis allocator reuse an existing client
// (e.g. the cache pillar's) instead of dialing its own, sharing the connection
// pool. The injected client is not closed on Stop — its owner keeps that
// responsibility. Ignored when WithAllocator is set.
func WithRedisClient(client *redis.Client) Option {
	return func(m *Manager) { m.injectedClient = client }
}

var (
	_ atlas.Pillar        = (*Manager)(nil)
	_ atlas.Starter       = (*Manager)(nil)
	_ atlas.HealthChecker = (*Manager)(nil)
)

func (m *Manager) Name() string { return "snowflake" }

func (m *Manager) Init(core *atlas.Core) error {
	var cfg Config
	if err := core.Unmarshal("snowflake", &cfg); err != nil {
		return fmt.Errorf("snowflake: %w", err)
	}
	cfg = cfg.withDefaults()
	if err := cfg.validate(m.allocator != nil); err != nil {
		return err
	}

	m.logger = core.Logger("snowflake")
	if m.logger == nil {
		m.logger = log.Global()
	}
	m.failSafe = cfg.FailMode != "besteffort"
	m.ttl, m.renew, m.safety = cfg.TTL, cfg.RenewInterval, cfg.SafetyMargin

	// An injected allocator (WithAllocator) wins; it is treated as lease-based so
	// the renewer/watchdog run (a static-style allocator's Renew is a harmless
	// no-op). Otherwise build from config: static worker_id or a Redis allocator
	// that reuses an injected client (WithRedisClient) or dials its own.
	switch {
	case m.allocator != nil:
		// use as-is
	case cfg.WorkerID != nil:
		m.static = true
		m.allocator = &staticAllocator{workerID: *cfg.WorkerID}
	default:
		alloc, err := newRedisAllocator(cfg, m.injectedClient)
		if err != nil {
			return err
		}
		m.allocator = alloc
	}

	t0 := time.Now()
	wid, err := m.allocator.Acquire(context.Background())
	if err != nil {
		return fmt.Errorf("snowflake: %w", err)
	}
	sf, err := id.NewSnowflake(wid)
	if err != nil {
		return fmt.Errorf("snowflake: %w", err)
	}
	m.gen = &Generator{}
	m.gen.setSnowflake(sf)
	m.gen.setOpen(true)
	// Anchor the lease estimate to before Acquire so it is never longer than the
	// real Redis TTL (see Manager.tryRenew).
	m.setLease(t0.Add(cfg.TTL))

	mode := "static"
	if !m.static {
		mode = "redis"
	}
	m.logger.Info(context.Background(), "snowflake worker id acquired",
		log.F("worker_id", wid), log.F("mode", mode))
	return nil
}

// Start launches lease renewal for the Redis allocator. Static allocation has
// nothing to renew.
func (m *Manager) Start(_ context.Context) error {
	if m.static {
		return nil
	}
	loopCtx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// renewer: extend the lease on schedule (bounded), re-acquire if lost.
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		t := time.NewTicker(m.renew)
		defer t.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-t.C:
				m.tryRenew(loopCtx)
			}
		}
	}()

	// watchdog: close the gate on wall-clock before the lease expires, even if
	// a renewal is blocked. Runs at a fraction of the safety margin.
	watchEvery := m.safety / 2
	if watchEvery <= 0 {
		watchEvery = time.Second
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		t := time.NewTicker(watchEvery)
		defer t.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-t.C:
				m.checkLease(time.Now())
			}
		}
	}()
	return nil
}

// Stop stops renewal and releases the worker ID.
func (m *Manager) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait() // ensure no renewal is in flight before releasing
	if m.allocator == nil {
		return nil
	}
	relErr := m.allocator.Release(ctx)
	closeErr := m.allocator.Close()
	if relErr != nil {
		return relErr
	}
	return closeErr
}

// Health reports unhealthy when the fail-safe gate is closed (lease lost),
// surfacing on /healthz (503) for monitoring. By design this does NOT drain
// /readyz: a shared-Redis blip loses every replica's lease at once, so draining
// all replicas together would be the exact cascade the readiness split avoids.
// Duplicate IDs are prevented by the gate itself (Generate returns
// ErrUnavailable), not by draining traffic.
func (m *Manager) Health(_ context.Context) error {
	if m.gen == nil || !m.gen.open.Load() {
		return fmt.Errorf("snowflake: generation unavailable (worker id lease lost)")
	}
	return nil
}
