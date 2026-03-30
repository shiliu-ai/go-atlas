package atlas

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"

	"github.com/shiliu-ai/go-atlas/atlas/i18n"
	"github.com/shiliu-ai/go-atlas/atlas/log"

	"golang.org/x/text/language"
)

// Atlas is the central framework instance that manages configuration,
// HTTP server, Pillars, middleware, and lifecycle.
type Atlas struct {
	name        string
	configName  string
	configPaths []string
	envPrefix   string

	config  *viper.Viper
	coreCfg coreConfig
	logger  log.Logger
	srv     *server

	registry *pillarRegistry

	extraMiddleware []gin.HandlerFunc
	skipDefaultMW   bool
	i18nBundle      *i18n.Bundle

	// rateLimitStore holds a reference to the rate limiter store (if any)
	// so its cleanup goroutine can be stopped during shutdown.
	rateLimitStore *memStore
}

// New creates and initializes an Atlas instance.
//
// Initialization flow:
//  1. Create Atlas with defaults
//  2. Apply all Options (registers Pillars and settings)
//  3. Load config (Viper)
//  4. Init Core (logger, server)
//  5. Init Pillars (in registration order)
//  6. Setup middleware (3 layers: core defaults -> pillar middleware -> user custom)
//  7. Register health routes
func New(name string, opts ...Option) *Atlas {
	a := &Atlas{
		name:        name,
		configName:  "config",
		configPaths: []string{"."},
		envPrefix:   "APP",
		registry:    newPillarRegistry(),
	}

	// 1. Apply options (registers Pillars and collects settings).
	for _, opt := range opts {
		opt(a)
	}

	// 2. Load config.
	v, cfg, err := loadConfig(a.configName, a.configPaths, a.envPrefix)
	if err != nil {
		panic(fmt.Sprintf("atlas: %v", err))
	}
	a.config = v
	a.coreCfg = cfg

	// 3. Init logger.
	if a.logger == nil {
		var logOpts []log.Option
		if a.coreCfg.Log.Format == "json" {
			logOpts = append(logOpts, log.WithJSON())
		}
		a.logger = log.NewDefault(parseLogLevel(a.coreCfg.Log.Level), logOpts...)
	}
	log.SetGlobal(a.logger)

	// Init i18n bundle.
	a.initI18n()

	// Init HTTP server.
	a.srv = newServer(a.coreCfg.Server)

	// 4. Init Pillars (in registration order).
	core := newCore(a.config, a.logger)
	for _, p := range a.registry.Pillars() {
		if err := p.Init(core); err != nil {
			panic(fmt.Sprintf("atlas: init pillar %q: %v", p.Name(), err))
		}
	}

	// 5. Setup middleware (3 layers).
	a.setupMiddleware()

	// 6. Register health routes.
	a.registerHealthRoutes()

	return a
}

// Run starts all Pillar Starters, the HTTP server, and blocks until
// a shutdown signal is received.
func (a *Atlas) Run() error {
	return a.run(context.Background())
}

// MustRun starts the application and panics on error.
func (a *Atlas) MustRun() {
	if err := a.Run(); err != nil {
		panic(fmt.Sprintf("atlas: run: %v", err))
	}
}

// Route registers routes on the service's base group.
func (a *Atlas) Route(fn func(*gin.RouterGroup)) *Atlas {
	fn(a.srv.group())
	return a
}

// Engine returns the Gin engine for direct access.
func (a *Atlas) Engine() *gin.Engine {
	return a.srv.engine
}

// Unmarshal deserializes the config section at key into target.
// This delegates to the underlying Viper instance.
func (a *Atlas) Unmarshal(key string, target any) error {
	sub := a.config.Sub(key)
	if sub == nil {
		return fmt.Errorf("atlas: config section %q not found", key)
	}
	return sub.Unmarshal(target)
}

// Logger returns the initialized logger.
func (a *Atlas) Logger() log.Logger {
	return a.logger
}

// Register adds a Pillar to the registry. This is a convenience method
// that delegates to the underlying pillarRegistry.
// Note: Pillars registered after New() will NOT be auto-initialized;
// prefer using Pillar option functions for standard usage.
func (a *Atlas) Register(p Pillar) {
	a.registry.Register(p)
}

// Use retrieves a registered Pillar by concrete type.
// Panics if no Pillar of the given type is found.
func Use[T Pillar](a *Atlas) T {
	return usePillar[T](a.registry)
}

// TryUse retrieves a registered Pillar by concrete type without panicking.
// Returns the pillar and true if found, zero value and false otherwise.
func TryUse[T Pillar](a *Atlas) (T, bool) {
	return tryUsePillar[T](a.registry)
}

// I18nBundle returns the i18n bundle for registering custom translations.
func (a *Atlas) I18nBundle() *i18n.Bundle {
	return a.i18nBundle
}

// --- internal helpers ---

// initI18n creates the i18n bundle with default translations and sets it as global.
func (a *Atlas) initI18n() {
	defaultLang := language.English
	if a.coreCfg.I18n.Default != "" {
		if tag, err := language.Parse(a.coreCfg.I18n.Default); err == nil {
			defaultLang = tag
		}
	}
	a.i18nBundle = i18n.NewBundle(defaultLang)
	i18n.RegisterDefaults(a.i18nBundle)
	i18n.SetGlobal(a.i18nBundle)
}

func parseLogLevel(s string) log.Level {
	switch s {
	case "debug":
		return log.LevelDebug
	case "warn":
		return log.LevelWarn
	case "error":
		return log.LevelError
	default:
		return log.LevelInfo
	}
}
