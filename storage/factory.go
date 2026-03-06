package storage

import (
	"context"
	"fmt"
	"sync"
)

const DefaultName = "default"

// Config is the unified storage configuration.
type Config struct {
	Driver string    `mapstructure:"driver"` // "s3", "cos", "oss", "tos"
	S3     S3Config  `mapstructure:"s3"`
	COS    COSConfig `mapstructure:"cos"`
	OSS    OSSConfig `mapstructure:"oss"`
	TOS    TOSConfig `mapstructure:"tos"`
}

// New creates a Storage instance based on the driver specified in Config.
func New(ctx context.Context, cfg Config) (Storage, error) {
	switch cfg.Driver {
	case "s3":
		return NewS3(ctx, cfg.S3)
	case "cos":
		return NewCOS(cfg.COS)
	case "oss":
		return NewOSS(cfg.OSS)
	case "tos":
		return NewTOS(cfg.TOS)
	default:
		return nil, fmt.Errorf("storage: unsupported driver %q", cfg.Driver)
	}
}

// Manager manages multiple named storage instances with lazy initialization.
type Manager struct {
	configs map[string]Config

	mu     sync.RWMutex
	stores map[string]Storage
}

// NewManager creates a Manager from a map of named configs.
func NewManager(configs map[string]Config) *Manager {
	return &Manager{
		configs: configs,
		stores:  make(map[string]Storage, len(configs)),
	}
}

// Get returns the named storage instance, initializing it on first access.
func (m *Manager) Get(name string) (Storage, error) {
	// Fast path: already initialized.
	m.mu.RLock()
	if s, ok := m.stores[name]; ok {
		m.mu.RUnlock()
		return s, nil
	}
	m.mu.RUnlock()

	// Slow path: initialize.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock.
	if s, ok := m.stores[name]; ok {
		return s, nil
	}

	cfg, ok := m.configs[name]
	if !ok {
		return nil, fmt.Errorf("storage: unknown instance %q", name)
	}

	s, err := New(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("storage: init %q: %w", name, err)
	}

	m.stores[name] = s
	return s, nil
}

// Default returns the "default" storage instance.
func (m *Manager) Default() (Storage, error) {
	return m.Get(DefaultName)
}

// Names returns all configured storage names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// Component wraps a Storage instance for app lifecycle integration.
type Component struct {
	Store Storage
}

func (c *Component) Name() string { return "storage" }
