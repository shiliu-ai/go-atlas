package sms

import (
	"context"
	"fmt"
	"sync"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Manager manages multiple named SMS instances with lazy initialization.
type Manager struct {
	configs map[string]Config

	mu       sync.RWMutex
	services map[string]SMS
}

// Get returns the named SMS instance, initializing it on first access.
func (m *Manager) Get(name string) (SMS, error) {
	// Fast path: already initialized.
	m.mu.RLock()
	if s, ok := m.services[name]; ok {
		m.mu.RUnlock()
		return s, nil
	}
	m.mu.RUnlock()

	// Slow path: initialize.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock.
	if s, ok := m.services[name]; ok {
		return s, nil
	}

	cfg, ok := m.configs[name]
	if !ok {
		return nil, fmt.Errorf("sms: unknown instance %q", name)
	}

	s, err := newSMS(cfg)
	if err != nil {
		return nil, fmt.Errorf("sms: init %q: %w", name, err)
	}

	m.services[name] = s
	return s, nil
}

// Default returns the "default" SMS instance.
func (m *Manager) Default() (SMS, error) {
	return m.Get(DefaultName)
}

// Names returns all configured SMS instance names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// Pillar returns an atlas.Option that registers the SMS Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		mgr := &Manager{}
		for _, opt := range opts {
			opt(mgr)
		}
		a.Register(mgr)
	}
}

// Of retrieves the SMS Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
	return atlas.Use[*Manager](a)
}

// Option configures the SMS Pillar.
type Option func(*Manager)

// Ensure interface compliance.
var _ atlas.Pillar = (*Manager)(nil)
var _ atlas.HealthChecker = (*Manager)(nil)

func (m *Manager) Name() string { return "sms" }

func (m *Manager) Init(core *atlas.Core) error {
	var cfg map[string]Config
	if err := core.Unmarshal("sms", &cfg); err != nil {
		return fmt.Errorf("sms: %w", err)
	}
	m.configs = cfg
	m.services = make(map[string]SMS, len(cfg))
	return nil
}

func (m *Manager) Stop(_ context.Context) error {
	return nil
}

func (m *Manager) Health(ctx context.Context) error {
	svc, err := m.Default()
	if err != nil {
		return fmt.Errorf("sms: health: %w", err)
	}
	return svc.Ping(ctx)
}
