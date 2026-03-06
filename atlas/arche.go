package atlas

import (
	"context"
	"fmt"
	"sync"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"gorm.io/gorm"

	"github.com/shiliu-ai/go-atlas/app"
	"github.com/shiliu-ai/go-atlas/auth"
	"github.com/shiliu-ai/go-atlas/cache"
	"github.com/shiliu-ai/go-atlas/config"
	"github.com/shiliu-ai/go-atlas/database"
	"github.com/shiliu-ai/go-atlas/httpclient"
	"github.com/shiliu-ai/go-atlas/log"
	"github.com/shiliu-ai/go-atlas/middleware"
	"github.com/shiliu-ai/go-atlas/server"
	"github.com/shiliu-ai/go-atlas/storage"
	"github.com/shiliu-ai/go-atlas/tracing"
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

	// lazy-initialized components
	once struct {
		auth       sync.Once
		db         sync.Once
		redis      sync.Once
		storage    sync.Once
		httpClient sync.Once
	}
	auth       *auth.JWT
	db         *gorm.DB
	dbErr      error
	redis      *cache.RedisCache
	redisErr   error
	store      storage.Storage
	storeErr   error
	httpClient *httpclient.Client

	// customCfg is an optional user-provided struct pointer for extra config fields.
	customCfg any
}

// New creates and initializes an Atlas instance.
// It loads configuration, initializes the logger, tracing, HTTP server,
// and registers default middleware automatically.
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
			logOpts = append(logOpts, log.WithFormat(log.FormatJSON))
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

	// Initialize HTTP server.
	a.server = server.New(a.cfg.Server)

	// Register middleware.
	if !a.skipDefaultMW {
		a.registerDefaultMiddleware()
	}
	if len(a.extraMiddleware) > 0 {
		a.server.Engine().Use(a.extraMiddleware...)
	}

	return a
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

// Auth returns the JWT instance (lazy-initialized).
func (a *Atlas) Auth() *auth.JWT {
	a.once.auth.Do(func() {
		a.auth = auth.New(a.cfg.Auth)
	})
	return a.auth
}

// DB returns the database engine (lazy-initialized).
func (a *Atlas) DB() (*gorm.DB, error) {
	a.once.db.Do(func() {
		a.db, a.dbErr = database.New(a.cfg.Database)
	})
	return a.db, a.dbErr
}

// Redis returns the Redis client (lazy-initialized).
func (a *Atlas) Redis() (*cache.RedisCache, error) {
	a.once.redis.Do(func() {
		a.redis, a.redisErr = cache.NewRedis(a.cfg.Redis)
	})
	return a.redis, a.redisErr
}

// Storage returns the object storage client (lazy-initialized).
func (a *Atlas) Storage() (storage.Storage, error) {
	a.once.storage.Do(func() {
		a.store, a.storeErr = storage.New(context.Background(), a.cfg.Storage)
	})
	return a.store, a.storeErr
}

// HTTPClient returns the HTTP client (lazy-initialized).
func (a *Atlas) HTTPClient() *httpclient.Client {
	a.once.httpClient.Do(func() {
		a.httpClient = httpclient.New(a.cfg.HTTPClient, a.logger)
	})
	return a.httpClient
}

// Route registers routes on the service's base group (/{name}).
// This is a convenience method for chaining.
func (a *Atlas) Route(fn func(*gin.RouterGroup)) *Atlas {
	fn(a.server.Group())
	return a
}

// Run starts the application and blocks until shutdown signal.
func (a *Atlas) Run() error {
	ap := app.New(a.name, a.logger)
	ap.Register(&server.Component{Server: a.server})

	// Register cleanup hooks.
	if a.tracingShutdown != nil {
		ap.OnShutdown(a.tracingShutdown)
	}
	if a.db != nil {
		ap.OnShutdown(func(ctx context.Context) error {
			sqlDB, err := a.db.DB()
			if err != nil {
				return err
			}
			return sqlDB.Close()
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
func (a *Atlas) MustRun() {
	if err := a.Run(); err != nil {
		panic(fmt.Sprintf("atlas: run: %v", err))
	}
}

func (a *Atlas) registerDefaultMiddleware() {
	mw := []gin.HandlerFunc{
		middleware.Recovery(a.logger),
		middleware.RequestID(),
	}

	// Add OTel middleware if tracing is configured.
	if a.cfg.Tracing.ServiceName != "" {
		mw = append(mw,
			otelgin.Middleware(a.cfg.Tracing.ServiceName),
			middleware.Tracing(),
		)
	}

	mw = append(mw, middleware.Logging(a.logger))

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
