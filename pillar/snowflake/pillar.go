package snowflake

import (
	"context"
	"fmt"
	"time"

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
	if err := cfg.validate(); err != nil {
		return err
	}

	m.logger = core.Logger("snowflake")
	if m.logger == nil {
		m.logger = log.Global()
	}
	m.failSafe = cfg.FailMode != "besteffort"
	m.ttl, m.renew, m.safety = cfg.TTL, cfg.RenewInterval, cfg.SafetyMargin

	if cfg.WorkerID != nil {
		m.static = true
		m.allocator = &staticAllocator{workerID: *cfg.WorkerID}
	} else {
		alloc, err := newRedisAllocator(cfg)
		if err != nil {
			return err
		}
		m.allocator = alloc
	}

	wid, err := m.allocator.Acquire(context.Background())
	if err != nil {
		return fmt.Errorf("snowflake: %w", err)
	}
	sf, err := id.NewSnowflake(wid)
	if err != nil {
		return fmt.Errorf("snowflake: %w", err)
	}
	m.gen = &Generator{sf: sf}
	m.gen.setOpen(true)
	m.leaseExpires = time.Now().Add(cfg.TTL)

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
	go func() {
		t := time.NewTicker(m.renew)
		defer t.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-t.C:
				m.renewOnce(loopCtx, time.Now())
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
	if m.allocator != nil {
		return m.allocator.Release(ctx)
	}
	return nil
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
