package database

import (
	"fmt"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/shiliu-ai/go-atlas/atlas/log"
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
	logger  log.Logger

	mu  sync.RWMutex
	dbs map[string]*gorm.DB
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

	db, err := openDB(cfg, m.logger)
	if err != nil {
		return nil, fmt.Errorf("database: init %q: %w", name, err)
	}

	m.dbs[name] = db
	return db, nil
}

// MustGet returns the named database connection or panics.
func (m *Manager) MustGet(name string) *gorm.DB {
	db, err := m.Get(name)
	if err != nil {
		panic(err)
	}
	return db
}

// Default returns the "default" database connection.
func (m *Manager) Default() (*gorm.DB, error) {
	return m.Get(DefaultName)
}

// Names returns all configured connection names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		names = append(names, name)
	}
	return names
}

// openDB creates a new gorm.DB from config.
func openDB(cfg Config, l log.Logger) (*gorm.DB, error) {
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
	if l == nil {
		l = log.Global()
	}
	gormCfg.Logger = newGormLogger(l, lvl, 200*time.Millisecond)

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
