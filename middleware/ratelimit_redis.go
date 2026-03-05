package middleware

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/shiliu-ai/go-atlas/errors"
	"github.com/shiliu-ai/go-atlas/response"
)

// RateLimitRedisConfig holds Redis-based distributed rate limiter configuration.
type RateLimitRedisConfig struct {
	Client  *redis.Client
	// Rate is the number of allowed requests per window.
	Rate    int
	// Window is the time window for rate limiting.
	Window  time.Duration
	// Prefix is the Redis key prefix. Defaults to "rl:".
	Prefix  string
	// KeyFunc extracts the rate limit key from the request.
	// Defaults to client IP if nil.
	KeyFunc func(c *gin.Context) string
}

// sliding window counter script:
// Returns 1 if allowed, 0 if denied.
var rateLimitScript = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local current = tonumber(redis.call("GET", key) or "0")
if current < limit then
    redis.call("INCR", key)
    if current == 0 then
        redis.call("PEXPIRE", key, window)
    end
    return 1
end
return 0
`)

// RateLimitRedis returns a Redis-based distributed rate limiter middleware.
func RateLimitRedis(cfg RateLimitRedisConfig) gin.HandlerFunc {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *gin.Context) string { return c.ClientIP() }
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "rl:"
	}

	return func(c *gin.Context) {
		key := cfg.Prefix + cfg.KeyFunc(c)
		windowMs := strconv.FormatInt(cfg.Window.Milliseconds(), 10)
		rateStr := strconv.Itoa(cfg.Rate)

		result, err := rateLimitScript.Run(
			context.Background(),
			cfg.Client,
			[]string{key},
			rateStr,
			windowMs,
		).Int64()

		if err != nil || result == 0 {
			response.Fail(c, errors.CodeTooManyRequests, "rate limit exceeded")
			c.Abort()
			return
		}
		c.Next()
	}
}
