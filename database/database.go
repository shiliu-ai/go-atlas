package database

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const DefaultName = "default"

// Config holds database configuration.
type Config struct {
	Driver       string        `mapstructure:"driver"` // mysql, postgres
	DSN          string        `mapstructure:"dsn"`
	MaxOpenConns int           `mapstructure:"max_open_conns"`
	MaxIdleConns int           `mapstructure:"max_idle_conns"`
	MaxLifetime  time.Duration `mapstructure:"max_lifetime"`
	ShowSQL      bool          `mapstructure:"show_sql"`
	LogLevel     string        `mapstructure:"log_level"`
}

// Manager manages multiple named database connections with lazy initialization.
type Manager struct {
	configs map[string]Config

	mu  sync.RWMutex
	dbs map[string]*gorm.DB
}

// NewManager creates a Manager from a map of named configs.
func NewManager(configs map[string]Config) *Manager {
	return &Manager{
		configs: configs,
		dbs:     make(map[string]*gorm.DB, len(configs)),
	}
}

// Get returns the named database connection, initializing it on first access.
func (m *Manager) Get(name string) (*gorm.DB, error) {
	// Fast path: already initialized.
	m.mu.RLock()
	if db, ok := m.dbs[name]; ok {
		m.mu.RUnlock()
		return db, nil
	}
	m.mu.RUnlock()

	// Slow path: initialize.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock.
	if db, ok := m.dbs[name]; ok {
		return db, nil
	}

	cfg, ok := m.configs[name]
	if !ok {
		return nil, fmt.Errorf("database: unknown connection %q", name)
	}

	db, err := New(cfg)
	if err != nil {
		return nil, fmt.Errorf("database: init %q: %w", name, err)
	}

	m.dbs[name] = db
	return db, nil
}

// Default returns the "default" database connection.
func (m *Manager) Default() (*gorm.DB, error) {
	return m.Get(DefaultName)
}

// Close closes all established database connections.
func (m *Manager) Close() error {
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

// Names returns all configured connection names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// New creates a new gorm.DB from config.
func New(cfg Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("database: unsupported driver %q", cfg.Driver)
	}

	gormCfg := &gorm.Config{}

	// Map log level.
	lvl := logger.Silent
	switch cfg.LogLevel {
	case "debug", "info":
		lvl = logger.Info
	case "warn":
		lvl = logger.Warn
	case "error":
		lvl = logger.Error
	}
	if cfg.ShowSQL && lvl > logger.Info {
		lvl = logger.Info
	}
	gormCfg.Logger = logger.Default.LogMode(lvl)

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: get sql.DB: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.MaxLifetime)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	return db, nil
}
