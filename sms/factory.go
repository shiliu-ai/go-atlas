package sms

import (
	"fmt"
	"sync"
)

const DefaultName = "default"

// Config is the unified SMS configuration.
type Config struct {
	Driver  string        `mapstructure:"driver"` // "tencentcloud"
	Tencent TencentConfig `mapstructure:"tencent"`
}

// TencentConfig holds Tencent Cloud SMS configuration.
type TencentConfig struct {
	SecretID  string `mapstructure:"secret_id"`
	SecretKey string `mapstructure:"secret_key"`
	AppID     string `mapstructure:"app_id"` // SmsSdkAppId
	Sign      string `mapstructure:"sign"`   // Default SMS signature
	Region    string `mapstructure:"region"` // Default: "ap-guangzhou"
}

// New creates an SMS instance based on the driver specified in Config.
func New(cfg Config) (SMS, error) {
	switch cfg.Driver {
	case "tencentcloud":
		return NewTencent(cfg.Tencent)
	default:
		return nil, fmt.Errorf("sms: unsupported driver %q", cfg.Driver)
	}
}

// Manager manages multiple named SMS instances with lazy initialization.
type Manager struct {
	configs map[string]Config

	mu       sync.RWMutex
	services map[string]SMS
}

// NewManager creates a Manager from a map of named configs.
func NewManager(configs map[string]Config) *Manager {
	return &Manager{
		configs:  configs,
		services: make(map[string]SMS, len(configs)),
	}
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

	s, err := New(cfg)
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

// Component wraps an SMS instance for app lifecycle integration.
type Component struct {
	SMS SMS
}

func (c *Component) Name() string { return "sms" }
