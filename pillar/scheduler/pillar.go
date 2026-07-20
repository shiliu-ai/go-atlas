package scheduler

import (
	"context"

	"github.com/robfig/cron/v3"

	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/atlas"
)

// Pillar returns an atlas.Option that registers the scheduler Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		m := &Manager{}
		for _, opt := range opts {
			opt(m)
		}
		a.Register(m)
	}
}

// Of retrieves the scheduler Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager { return atlas.Use[*Manager](a) }

// Option configures the scheduler Pillar.
type Option func(*Manager)

var (
	_ atlas.Pillar  = (*Manager)(nil)
	_ atlas.Starter = (*Manager)(nil)
)

func (m *Manager) Name() string { return "scheduler" }

func (m *Manager) Init(core *atlas.Core) error {
	m.logger = core.Logger("scheduler")
	if m.logger == nil {
		m.logger = log.Global()
	}
	if m.cron == nil {
		m.cron = cron.New()
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	return nil
}

// Start begins executing scheduled jobs. Non-blocking (cron runs its own
// goroutine).
func (m *Manager) Start(_ context.Context) error {
	m.cron.Start()
	return nil
}

// Stop halts scheduling, signals in-flight jobs to cancel (via the context
// passed to their Run), and waits for them to finish, bounded by ctx.
func (m *Manager) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	stopCtx := m.cron.Stop() // stops scheduling; done when running jobs finish
	select {
	case <-stopCtx.Done():
	case <-ctx.Done():
	}
	return nil
}
