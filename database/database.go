package database

import (
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"xorm.io/xorm"
	"xorm.io/xorm/log"
)

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

// New creates a new xorm.Engine from config.
func New(cfg Config) (*xorm.Engine, error) {
	engine, err := xorm.NewEngine(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("database: new engine: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		engine.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		engine.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.MaxLifetime > 0 {
		engine.SetConnMaxLifetime(cfg.MaxLifetime)
	}

	engine.ShowSQL(cfg.ShowSQL)

	switch cfg.LogLevel {
	case "debug":
		engine.Logger().SetLevel(log.LOG_DEBUG)
	case "info":
		engine.Logger().SetLevel(log.LOG_INFO)
	case "warn":
		engine.Logger().SetLevel(log.LOG_WARNING)
	case "error":
		engine.Logger().SetLevel(log.LOG_ERR)
	}

	if err := engine.Ping(); err != nil {
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	return engine, nil
}
