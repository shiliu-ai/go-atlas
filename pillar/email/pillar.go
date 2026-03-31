package email

import (
	"context"
	"fmt"
	"sync"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Manager manages multiple named Email instances with lazy initialization.
type Manager struct {
	configs map[string]Config

	mu       sync.RWMutex
	services map[string]Email
}

// Get returns the named Email instance, initializing it on first access.
func (m *Manager) Get(name string) (Email, error) {
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
		return nil, fmt.Errorf("email: unknown instance %q", name)
	}

	s, err := newEmail(cfg)
	if err != nil {
		return nil, fmt.Errorf("email: init %q: %w", name, err)
	}

	m.services[name] = s
	return s, nil
}

// Default returns the "default" Email instance.
func (m *Manager) Default() (Email, error) {
	return m.Get(DefaultName)
}

// Names returns all configured Email instance names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// Pillar returns an atlas.Option that registers the Email Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		mgr := &Manager{}
		for _, opt := range opts {
			opt(mgr)
		}
		a.Register(mgr)
	}
}

// Of retrieves the Email Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
	return atlas.Use[*Manager](a)
}

// Option configures the Email Pillar.
type Option func(*Manager)

// Ensure interface compliance at compile time.
var _ atlas.Pillar = (*Manager)(nil)
var _ atlas.HealthChecker = (*Manager)(nil)

func (m *Manager) Name() string { return "email" }

func (m *Manager) Init(core *atlas.Core) error {
	var cfg map[string]Config
	if err := core.Unmarshal("email", &cfg); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	m.configs = cfg
	m.services = make(map[string]Email, len(cfg))
	return nil
}

func (m *Manager) Stop(_ context.Context) error {
	return nil
}

func (m *Manager) Health(ctx context.Context) error {
	svc, err := m.Default()
	if err != nil {
		return fmt.Errorf("email: health: %w", err)
	}
	return svc.Ping(ctx)
}
