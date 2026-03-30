package oauth

import (
	"context"
	"fmt"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Manager manages multiple named OAuth2 providers.
type Manager struct {
	providers map[string]*Provider
}

// Get returns the named OAuth2 provider.
func (m *Manager) Get(name string) (*Provider, error) {
	p, ok := m.providers[name]
	if !ok {
		return nil, fmt.Errorf("oauth: provider %q not configured", name)
	}
	return p, nil
}

// MustGet returns the named OAuth2 provider or panics.
func (m *Manager) MustGet(name string) *Provider {
	p, err := m.Get(name)
	if err != nil {
		panic(err)
	}
	return p
}

// Names returns all configured provider names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// Pillar returns an atlas.Option that registers the oauth Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		m := &Manager{}
		for _, opt := range opts {
			opt(m)
		}
		a.Register(m)
	}
}

// Of retrieves the oauth Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
	return atlas.Use[*Manager](a)
}

// Option configures the oauth Pillar.
type Option func(*Manager)

// Ensure interface compliance.
var _ atlas.Pillar = (*Manager)(nil)

func (m *Manager) Name() string { return "oauth" }

func (m *Manager) Init(core *atlas.Core) error {
	var cfg map[string]ProviderConfig
	if err := core.Unmarshal("oauth", &cfg); err != nil {
		return fmt.Errorf("oauth: %w", err)
	}
	m.providers = make(map[string]*Provider, len(cfg))
	for name, pc := range cfg {
		m.providers[name] = NewProvider(name, pc)
	}
	return nil
}

func (m *Manager) Stop(_ context.Context) error {
	return nil
}
