package atlas

import (
	"context"
	"net/http"
	"net/http/httputil"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shiliu-ai/go-atlas/aether/errors"
	"github.com/shiliu-ai/go-atlas/aether/i18n"
	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/aether/response"
)

// --- Middleware setup ---

// setupMiddleware assembles 3 layers:
//  1. Core defaults (Recovery, RequestID, I18n, Logging, CORS, RateLimit if configured)
//  2. Pillar middleware (from MiddlewareProvider interface)
//  3. User custom middleware (WithMiddleware)
func (a *Atlas) setupMiddleware() {
	if !a.skipDefaultMW {
		mw := a.coreMiddleware()
		a.srv.engine.Use(mw...)
	}

	// Pillar middleware.
	for _, p := range a.registry.Pillars() {
		if mp, ok := p.(MiddlewareProvider); ok {
			a.srv.engine.Use(mp.Middleware()...)
		}
	}

	// User custom middleware.
	if len(a.extraMiddleware) > 0 {
		a.srv.engine.Use(a.extraMiddleware...)
	}
}

// coreMiddleware returns the default middleware chain.
func (a *Atlas) coreMiddleware() []gin.HandlerFunc {
	mw := []gin.HandlerFunc{
		recoveryMiddleware(a.logger),
		requestIDMiddleware(),
		i18n.Middleware(a.i18nBundle),
		loggingMiddleware(a.logger),
	}

	// CORS: use config if provided, otherwise use defaults.
	corsConfig := a.coreCfg.Middleware.CORS
	if len(corsConfig.AllowOrigins) == 0 {
		corsConfig = defaultCORSConfig()
	}
	mw = append(mw, corsMiddleware(corsConfig))

	// Rate limit: prefer an injected limiter (e.g. distributed Redis), else
	// fall back to the local in-memory limiter when configured.
	var limiter RateLimiter
	if a.rateLimiter != nil {
		limiter = a.rateLimiter
	} else if a.coreCfg.Middleware.RateLimit.Rate > 0 {
		window := a.coreCfg.Middleware.RateLimit.Window
		if window <= 0 {
			// A zero window would reset the bucket every request (allow all).
			// Default to 1 minute and warn rather than silently disable limiting.
			window = time.Minute
			a.logger.Warn(context.Background(), "rate_limit.window unset or <=0; defaulting to 1m",
				log.F("rate", a.coreCfg.Middleware.RateLimit.Rate))
		}
		store := &memStore{
			buckets: make(map[string]*rlBucket),
			rate:    a.coreCfg.Middleware.RateLimit.Rate,
			window:  window,
			done:    make(chan struct{}),
		}
		a.rateLimitStore = store
		go store.cleanup()
		limiter = store
	}
	if limiter != nil {
		mw = append(mw, rateLimitMiddleware(limiter, nil, a.logger))
	}

	return mw
}

// --- Recovery middleware ---

// recoveryMiddleware recovers from panics and logs the stack trace.
func recoveryMiddleware(logger log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := string(debug.Stack())

				// Try to capture the request for debugging.
				var reqDump string
				if c.Request != nil {
					if dump, err := httputil.DumpRequest(c.Request, false); err == nil {
						reqDump = string(dump)
					}
				}

				fields := []log.Field{
					log.F("error", r),
					log.F("stack", stack),
				}
				if reqDump != "" {
					fields = append(fields, log.F("request", reqDump))
				}

				ctx := context.Background()
				if c.Request != nil {
					ctx = c.Request.Context()
				}
				logger.Error(ctx, "panic recovered", fields...)
				c.AbortWithStatusJSON(http.StatusInternalServerError,
					response.NewR(c, int(errors.CodeInternal), "internal server error", nil),
				)
			}
		}()
		c.Next()
	}
}

// --- RequestID middleware ---

const headerRequestID = "X-Request-ID"

// requestIDMiddleware injects a unique request ID into context and response header.
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(headerRequestID)
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Header(headerRequestID, requestID)

		ctx := log.WithRequestID(c.Request.Context(), requestID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// --- Logging middleware ---

// loggingMiddleware logs each HTTP request with duration, status, and method.
func loggingMiddleware(logger log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		if raw != "" {
			path = path + "?" + raw
		}

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []log.Field{
			log.F("status", status),
			log.F("method", c.Request.Method),
			log.F("path", path),
			log.F("latency_ms", float64(latency)/float64(time.Millisecond)),
			log.F("ip", c.ClientIP()),
		}

		if len(c.Errors) > 0 {
			logger.Error(c.Request.Context(), c.Errors.String(), fields...)
		} else if status >= 500 {
			logger.Error(c.Request.Context(), "server error", fields...)
		} else if status >= 400 {
			logger.Warn(c.Request.Context(), "client error", fields...)
		} else {
			logger.Info(c.Request.Context(), "request", fields...)
		}
	}
}

// --- CORS middleware ---

// corsConfig holds CORS configuration.
type corsConfig struct {
	AllowOrigins []string `mapstructure:"allow_origins"`
	AllowMethods []string `mapstructure:"allow_methods"`
	AllowHeaders []string `mapstructure:"allow_headers"`
	MaxAge       int      `mapstructure:"max_age"`
}

// defaultCORSConfig returns a permissive CORS config for development.
func defaultCORSConfig() corsConfig {
	return corsConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
		MaxAge:       86400,
	}
}

// corsMiddleware returns a CORS middleware.
func corsMiddleware(cfg corsConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		allowed := false
		for _, o := range cfg.AllowOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}
		if !allowed {
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ","))
		c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ","))
		c.Header("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
		if origin != "*" {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// --- Rate limit middleware (in-memory) ---

// rateLimitTimeout bounds each limiter backend call so a hung backend cannot
// pin request goroutines; on timeout the middleware fails open. Var (not const)
// for tests.
var rateLimitTimeout = time.Second

// RateLimiter reports whether a request identified by key may proceed.
// Implementations may be local (in-process) or distributed (e.g. Redis via
// pillar/ratelimit). Inject a distributed limiter with atlas.WithRateLimiter.
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// rateLimitMiddleware limits requests using the given RateLimiter, keyed by
// keyFunc (default: client IP). If the limiter backend errors it fails open
// (allows the request) so an unavailable backend cannot take down the service.
func rateLimitMiddleware(limiter RateLimiter, keyFunc func(c *gin.Context) string, logger log.Logger) gin.HandlerFunc {
	if keyFunc == nil {
		keyFunc = func(c *gin.Context) string { return c.ClientIP() }
	}

	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), rateLimitTimeout)
		defer cancel()
		allowed, err := limiter.Allow(ctx, keyFunc(c))
		if err != nil {
			logger.Warn(c.Request.Context(), "rate limiter backend error, failing open",
				log.F("error", err))
			c.Next()
			return
		}
		if !allowed {
			response.Fail(c, errors.CodeTooManyRequests, "rate limit exceeded")
			c.Abort()
			return
		}
		c.Next()
	}
}

type rlBucket struct {
	tokens    int
	lastReset time.Time
}

type memStore struct {
	mu      sync.Mutex
	buckets map[string]*rlBucket
	rate    int
	window  time.Duration
	done    chan struct{}
}

// Ensure memStore satisfies RateLimiter.
var _ RateLimiter = (*memStore)(nil)

// Allow implements RateLimiter for the local in-memory store (never errors).
func (s *memStore) Allow(_ context.Context, key string) (bool, error) {
	return s.allow(key), nil
}

func (s *memStore) allow(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	b, ok := s.buckets[key]
	if !ok || now.Sub(b.lastReset) >= s.window {
		s.buckets[key] = &rlBucket{tokens: s.rate - 1, lastReset: now}
		return true
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}

func (s *memStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for k, b := range s.buckets {
				if now.Sub(b.lastReset) > s.window*2 {
					delete(s.buckets, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

// stop terminates the cleanup goroutine.
func (s *memStore) stop() {
	close(s.done)
}
