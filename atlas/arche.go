package atlas

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"gorm.io/gorm"

	"github.com/shiliu-ai/go-atlas/app"
	"github.com/shiliu-ai/go-atlas/auth"
	"github.com/shiliu-ai/go-atlas/cache"
	"github.com/shiliu-ai/go-atlas/config"
	"github.com/shiliu-ai/go-atlas/database"
	"github.com/shiliu-ai/go-atlas/httpclient"
	"github.com/shiliu-ai/go-atlas/i18n"
	"github.com/shiliu-ai/go-atlas/log"
	"github.com/shiliu-ai/go-atlas/middleware"
	"github.com/shiliu-ai/go-atlas/server"
	"github.com/shiliu-ai/go-atlas/serviceclient"
	"github.com/shiliu-ai/go-atlas/storage"
	"github.com/shiliu-ai/go-atlas/tracing"

	"golang.org/x/text/language"
)

// Option configures the Atlas instance.
type Option func(*Atlas)

// WithConfigName sets the config file name (without extension). Default: "config".
func WithConfigName(name string) Option {
	return func(a *Atlas) { a.configName = name }
}

// WithConfigPaths sets the directories to search for the config file.
// Default: ["."].
func WithConfigPaths(paths ...string) Option {
	return func(a *Atlas) { a.configPaths = paths }
}

// WithEnvPrefix sets the environment variable prefix. Default: "APP".
func WithEnvPrefix(prefix string) Option {
	return func(a *Atlas) { a.envPrefix = prefix }
}

// WithLogger overrides the auto-created logger.
func WithLogger(l log.Logger) Option {
	return func(a *Atlas) { a.logger = l }
}

// WithMiddleware appends additional Gin middleware to the default stack.
func WithMiddleware(mw ...gin.HandlerFunc) Option {
	return func(a *Atlas) { a.extraMiddleware = append(a.extraMiddleware, mw...) }
}

// WithoutDefaultMiddleware disables all default middleware registration.
// Useful when the service wants full control over middleware ordering.
func WithoutDefaultMiddleware() Option {
	return func(a *Atlas) { a.skipDefaultMW = true }
}

// WithCustomConfig provides a custom config struct for loading.
// The struct should embed atlas.Config to include framework fields:
//
//	type MyConfig struct {
//	    atlas.Config `mapstructure:",squash"`
//	    Business     string `mapstructure:"business"`
//	}
//
//	var cfg MyConfig
//	a := atlas.New("svc", atlas.WithCustomConfig(&cfg))
//	// cfg.Business is now loaded from config file
func WithCustomConfig(target any) Option {
	return func(a *Atlas) { a.customCfg = target }
}

// Atlas provides a one-stop initialization for go-atlas based services.
type Atlas struct {
	name        string
	cfg         Config
	configName  string
	configPaths []string
	envPrefix   string

	logger          log.Logger
	server          *server.Server
	tracingShutdown func(context.Context) error

	extraMiddleware []gin.HandlerFunc
	skipDefaultMW   bool

	// i18n bundle (always initialized)
	i18nBundle *i18n.Bundle

	// eagerly initialized components (nil if not configured)
	auth       *auth.JWT
	dbm        *database.Manager
	redis      *cache.RedisCache
	stm        *storage.Manager
	httpClient *httpclient.Client
	svcm       *serviceclient.Manager

	// customCfg is an optional user-provided struct pointer for extra config fields.
	customCfg any
}

// New creates and initializes an Atlas instance.
// It loads configuration, initializes the logger, tracing, HTTP server,
// and registers default middleware automatically.
// Components with valid configuration (databases, redis, storages, etc.)
// are eagerly initialized; initialization failures cause a panic (fail-fast).
func New(name string, opts ...Option) *Atlas {
	a := &Atlas{
		name:        name,
		configName:  "config",
		configPaths: []string{"."},
		envPrefix:   "APP",
	}

	for _, opt := range opts {
		opt(a)
	}

	// Load config.
	target := a.customCfg
	if target == nil {
		target = &a.cfg
	}
	if err := config.Load(a.configName, a.configPaths, a.envPrefix, target); err != nil {
		panic(fmt.Sprintf("atlas: load config: %v", err))
	}

	// If custom config was used, try to extract the embedded Config.
	if a.customCfg != nil {
		a.cfg = extractConfig(a.customCfg)
	}

	// Initialize logger.
	if a.logger == nil {
		var logOpts []log.Option
		if a.cfg.Log.Format == "json" {
			logOpts = append(logOpts, log.WithJSON())
		}
		a.logger = log.NewDefault(parseLogLevel(a.cfg.Log.Level), logOpts...)
	}
	log.SetGlobal(a.logger)

	// Initialize tracing (non-fatal on error).
	if a.cfg.Tracing.Endpoint != "" {
		shutdown, err := tracing.Init(context.Background(), a.cfg.Tracing)
		if err != nil {
			a.logger.Error(context.Background(), "atlas: tracing init error", log.F("error", err))
		} else {
			a.tracingShutdown = shutdown
		}
	}

	// Initialize i18n bundle.
	a.initI18n()

	// Initialize HTTP server.
	a.server = server.New(a.cfg.Server)

	// Register middleware.
	if !a.skipDefaultMW {
		a.registerDefaultMiddleware()
	}
	if len(a.extraMiddleware) > 0 {
		a.server.Engine().Use(a.extraMiddleware...)
	}

	// Eagerly initialize configured components.
	a.initComponents()

	return a
}

// initComponents eagerly initializes all components that have valid configuration.
func (a *Atlas) initComponents() {
	if a.cfg.Auth.Secret != "" {
		a.auth = auth.New(a.cfg.Auth)
	}

	if len(a.cfg.Databases) > 0 {
		a.dbm = database.NewManager(a.cfg.Databases)
	}

	if a.cfg.Redis.Addr != "" {
		redis, err := cache.NewRedis(a.cfg.Redis)
		if err != nil {
			panic(fmt.Sprintf("atlas: init redis: %v", err))
		}
		a.redis = redis
	}

	if len(a.cfg.Storages) > 0 {
		a.stm = storage.NewManager(a.cfg.Storages)
	}

	a.httpClient = httpclient.New(a.cfg.HTTPClient, a.logger)

	if len(a.cfg.Services) > 0 {
		a.svcm = serviceclient.NewManager(a.cfg.Services, a.cfg.HTTPClient, a.logger)
	}
}

// Config returns the loaded framework configuration.
func (a *Atlas) Config() Config { return a.cfg }

// Logger returns the initialized logger.
func (a *Atlas) Logger() log.Logger { return a.logger }

// Server returns the underlying HTTP server.
func (a *Atlas) Server() *server.Server { return a.server }

// Engine returns the Gin engine for direct access.
func (a *Atlas) Engine() *gin.Engine { return a.server.Engine() }

// Group returns the service's base route group (/{name}).
func (a *Atlas) Group(middlewares ...gin.HandlerFunc) *gin.RouterGroup {
	return a.server.Group(middlewares...)
}

// Auth returns the JWT instance. Panics if auth is not configured.
func (a *Atlas) Auth() *auth.JWT {
	if a.auth == nil {
		panic("atlas: auth not configured (set auth.secret in config)")
	}
	return a.auth
}

// DB returns the default database connection.
// This is a convenience shortcut for DBManager().Default().
func (a *Atlas) DB() (*gorm.DB, error) {
	mgr := a.DBManager()
	return mgr.Default()
}

// DBManager returns the database manager for accessing named connections.
// Panics if no databases are configured.
func (a *Atlas) DBManager() *database.Manager {
	if a.dbm == nil {
		panic("atlas: no databases configured")
	}
	return a.dbm
}

// Redis returns the Redis client. Panics if redis is not configured.
func (a *Atlas) Redis() *cache.RedisCache {
	if a.redis == nil {
		panic("atlas: redis not configured (set redis.addr in config)")
	}
	return a.redis
}

// Storage returns the default storage instance.
// This is a convenience shortcut for StorageManager().Default().
func (a *Atlas) Storage() (storage.Storage, error) {
	mgr := a.StorageManager()
	return mgr.Default()
}

// StorageManager returns the storage manager for accessing named instances.
// Panics if no storages are configured.
func (a *Atlas) StorageManager() *storage.Manager {
	if a.stm == nil {
		panic("atlas: no storages configured")
	}
	return a.stm
}

// HTTPClient returns the HTTP client.
func (a *Atlas) HTTPClient() *httpclient.Client {
	return a.httpClient
}

// Service returns the client for the named upstream service.
// The returned Service interface can be mocked in tests.
// Panics if the service is not configured.
func (a *Atlas) Service(name string) serviceclient.Service {
	return a.ServiceManager().MustGet(name)
}

// ServiceManager returns the service client manager.
// Panics if no services are configured.
func (a *Atlas) ServiceManager() *serviceclient.Manager {
	if a.svcm == nil {
		panic("atlas: no services configured (set services in config)")
	}
	return a.svcm
}

// CustomConfigPtr returns the raw pointer to the custom config struct.
// Use the generic CustomConfig or MustCustomConfig functions for typed access.
func (a *Atlas) CustomConfigPtr() any {
	return a.customCfg
}

// CustomConfig returns the custom config struct with type assertion.
// Returns the typed config and true if the assertion succeeds, zero value and false otherwise.
func CustomConfig[T any](a *Atlas) (*T, bool) {
	cfg, ok := a.customCfg.(*T)
	return cfg, ok
}

// MustCustomConfig returns the custom config struct with type assertion.
// Panics if the custom config is nil or the type assertion fails.
func MustCustomConfig[T any](a *Atlas) *T {
	cfg, ok := CustomConfig[T](a)
	if !ok {
		panic(fmt.Sprintf("atlas: custom config type mismatch: want *%T", (*T)(nil)))
	}
	return cfg
}

// Route registers routes on the service's base group (/{name}).
// This is a convenience method for chaining.
func (a *Atlas) Route(fn func(*gin.RouterGroup)) *Atlas {
	fn(a.server.Group())
	return a
}

// Run starts the application and blocks until shutdown signal.
// Optional hooks are executed before the server starts (e.g. route registration).
func (a *Atlas) Run(hooks ...func(*Atlas)) error {
	for _, h := range hooks {
		h(a)
	}

	ap := app.New(a.name, a.logger)
	ap.Register(&server.Component{Server: a.server})

	// Register cleanup hooks.
	if a.tracingShutdown != nil {
		ap.OnShutdown(a.tracingShutdown)
	}
	if a.dbm != nil {
		ap.OnShutdown(func(ctx context.Context) error {
			return a.dbm.Close()
		})
	}
	if a.redis != nil {
		ap.OnShutdown(func(ctx context.Context) error {
			return a.redis.Close()
		})
	}

	return ap.Run(context.Background())
}

// MustRun starts the application and panics on error.
// Optional hooks are executed before the server starts (e.g. route registration).
func (a *Atlas) MustRun(hooks ...func(*Atlas)) {
	if err := a.Run(hooks...); err != nil {
		panic(fmt.Sprintf("atlas: run: %v", err))
	}
}

func (a *Atlas) registerDefaultMiddleware() {
	mw := []gin.HandlerFunc{
		middleware.Recovery(a.logger),
		middleware.RequestID(),
		i18n.Middleware(a.i18nBundle),
	}

	// Add OTel middleware if tracing is configured.
	if a.cfg.Tracing.ServiceName != "" {
		mw = append(mw,
			otelgin.Middleware(a.cfg.Tracing.ServiceName),
			middleware.Tracing(),
		)
	}

	mw = append(mw, middleware.Logging(a.logger))

	// Forward headers for inter-service communication.
	if len(a.cfg.Services) > 0 {
		mw = append(mw, serviceclient.ForwardHeaders())
	}

	// CORS: use config if provided, otherwise use defaults.
	corsConfig := a.cfg.Middleware.CORS
	if len(corsConfig.AllowOrigins) == 0 {
		corsConfig = middleware.DefaultCORSConfig()
	}
	mw = append(mw, middleware.CORS(corsConfig))

	// Rate limit: only add if configured.
	if a.cfg.Middleware.RateLimit.Rate > 0 {
		mw = append(mw, middleware.RateLimit(middleware.RateLimitConfig{
			Rate:   a.cfg.Middleware.RateLimit.Rate,
			Window: a.cfg.Middleware.RateLimit.Window,
		}))
	}

	a.server.Engine().Use(mw...)
}

// initI18n creates the i18n bundle with default translations and sets it as global.
func (a *Atlas) initI18n() {
	defaultLang := language.English
	if a.cfg.I18n.Default != "" {
		if tag, err := language.Parse(a.cfg.I18n.Default); err == nil {
			defaultLang = tag
		}
	}
	a.i18nBundle = i18n.NewBundle(defaultLang)
	i18n.RegisterDefaults(a.i18nBundle)
	i18n.SetGlobal(a.i18nBundle)
}

// I18nBundle returns the i18n bundle for registering custom translations.
func (a *Atlas) I18nBundle() *i18n.Bundle {
	return a.i18nBundle
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

// extractConfig extracts the embedded Config from a custom config struct.
// It uses reflection-free approach: the custom struct must embed Config directly.
func extractConfig(v any) Config {
	type configEmbedder interface {
		AtlasConfig() Config
	}
	if ce, ok := v.(configEmbedder); ok {
		return ce.AtlasConfig()
	}
	// Fallback: if user didn't embed, return zero config.
	// This shouldn't happen when used correctly.
	return Config{}
}
