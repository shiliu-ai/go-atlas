package atlas

import (
	"time"

	"github.com/shiliu-ai/go-atlas/auth"
	"github.com/shiliu-ai/go-atlas/cache"
	"github.com/shiliu-ai/go-atlas/database"
	"github.com/shiliu-ai/go-atlas/httpclient"
	"github.com/shiliu-ai/go-atlas/middleware"
	"github.com/shiliu-ai/go-atlas/server"
	"github.com/shiliu-ai/go-atlas/serviceclient"
	"github.com/shiliu-ai/go-atlas/sms"
	"github.com/shiliu-ai/go-atlas/storage"
	"github.com/shiliu-ai/go-atlas/tracing"
)

// Config aggregates all framework component configurations.
// Upper-level services can embed this struct to add custom fields:
//
//	type MyConfig struct {
//	    atlas.Config `mapstructure:",squash"`
//	    Business         BusinessConfig `mapstructure:"business"`
//	}
type Config struct {
	Server     server.Config     `mapstructure:"server"`
	Log        LogConfig         `mapstructure:"log"`
	Auth       auth.Config       `mapstructure:"auth"`
	Databases  map[string]database.Config `mapstructure:"databases"`
	Redis      cache.RedisConfig `mapstructure:"redis"`
	Storages   map[string]storage.Config `mapstructure:"storages"`
	SMS        map[string]sms.Config     `mapstructure:"sms"`
	Tracing    tracing.Config    `mapstructure:"tracing"`
	HTTPClient httpclient.Config                    `mapstructure:"httpclient"`
	Services   map[string]serviceclient.ServiceConfig `mapstructure:"services"`
	I18n       I18nConfig                               `mapstructure:"i18n"`
	Middleware MiddlewareConfig                       `mapstructure:"middleware"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // "text" (default) or "json"
}

// I18nConfig holds i18n configuration.
type I18nConfig struct {
	Default string `mapstructure:"default"` // Default language tag, e.g. "en", "zh-Hans". Default: "en".
}

// MiddlewareConfig holds middleware configuration.
type MiddlewareConfig struct {
	CORS      middleware.CORSConfig `mapstructure:"cors"`
	RateLimit RateLimitConfig       `mapstructure:"rate_limit"`
}

// RateLimitConfig holds rate limiter YAML-friendly configuration.
type RateLimitConfig struct {
	Rate   int           `mapstructure:"rate"`
	Window time.Duration `mapstructure:"window"`
}
