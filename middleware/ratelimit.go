package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shiliu-ai/go-atlas/errors"
	"github.com/shiliu-ai/go-atlas/response"
)

// RateLimitConfig holds rate limiter configuration.
type RateLimitConfig struct {
	// Rate is the number of allowed requests per window.
	Rate int
	// Window is the time window for rate limiting.
	Window time.Duration
	// KeyFunc extracts the rate limit key from the request.
	// Defaults to client IP if nil.
	KeyFunc func(c *gin.Context) string
}

// RateLimit returns a local in-memory token-bucket rate limiter middleware.
// For distributed rate limiting across multiple instances, use RateLimitRedis.
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *gin.Context) string { return c.ClientIP() }
	}

	store := &memoryStore{
		buckets: make(map[string]*bucket),
		rate:    cfg.Rate,
		window:  cfg.Window,
	}

	// Periodically clean up expired entries.
	go store.cleanup()

	return func(c *gin.Context) {
		key := cfg.KeyFunc(c)
		if !store.allow(key) {
			response.Fail(c, errors.CodeTooManyRequests, "rate limit exceeded")
			c.Abort()
			return
		}
		c.Next()
	}
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

type memoryStore struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    int
	window  time.Duration
}

func (s *memoryStore) allow(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	b, ok := s.buckets[key]
	if !ok || now.Sub(b.lastReset) >= s.window {
		s.buckets[key] = &bucket{tokens: s.rate - 1, lastReset: now}
		return true
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}

func (s *memoryStore) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
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
