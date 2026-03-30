package database

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Pillar returns an atlas.Option that registers the database Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		m := &Manager{}
		for _, opt := range opts {
			opt(m)
		}
		a.Register(m)
	}
}

// Of retrieves the database Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
	return atlas.Use[*Manager](a)
}

// Option configures the database Pillar.
type Option func(*Manager)

// Ensure interface compliance.
var _ atlas.Pillar = (*Manager)(nil)
var _ atlas.HealthChecker = (*Manager)(nil)

func (m *Manager) Name() string { return "databases" }

func (m *Manager) Init(core *atlas.Core) error {
	var cfg map[string]Config
	if err := core.Unmarshal("databases", &cfg); err != nil {
		return fmt.Errorf("databases: %w", err)
	}
	m.configs = cfg
	m.dbs = make(map[string]*gorm.DB, len(cfg))
	m.logger = core.Logger("database")
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, db := range m.dbs {
		sqlDB, err := db.DB()
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("database: close %q: %w", name, err)
			}
			continue
		}
		if err := sqlDB.Close(); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("database: close %q: %w", name, err)
			}
		}
	}
	m.dbs = make(map[string]*gorm.DB)
	return firstErr
}

func (m *Manager) Health(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, db := range m.dbs {
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("database: health %q: %w", name, err)
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("database: health %q: %w", name, err)
		}
	}
	return nil
}
