package storage

import (
	"context"
	"fmt"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Pillar returns an atlas.Option that registers the storage Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		m := &Manager{}
		for _, opt := range opts {
			opt(m)
		}
		a.Register(m)
	}
}

// Of retrieves the storage Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
	return atlas.Use[*Manager](a)
}

// Option configures the storage Pillar.
type Option func(*Manager)

// Ensure interface compliance.
var _ atlas.Pillar = (*Manager)(nil)

func (m *Manager) Name() string { return "storages" }

func (m *Manager) Init(core *atlas.Core) error {
	var cfg map[string]Config
	if err := core.Unmarshal("storages", &cfg); err != nil {
		return fmt.Errorf("storages: %w", err)
	}
	m.configs = cfg
	m.stores = make(map[string]Storage, len(cfg))
	return nil
}

func (m *Manager) Stop(_ context.Context) error {
	return nil
}
