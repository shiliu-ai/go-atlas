// Package ratelimit provides a Redis-backed distributed RateLimiter for the
// atlas framework, so multiple service replicas share a single quota. Wire it
// with atlas.WithRateLimiter.
package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// fixedWindow atomically increments a per-key counter and sets its expiry on
// first use, allowing up to `limit` requests per window. Returns 1 to allow,
// 0 to deny.
var fixedWindow = redis.NewScript(`
local c = redis.call("INCR", KEYS[1])
if c == 1 then
    redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
if c > tonumber(ARGV[2]) then
    return 0
end
return 1
`)

// Config configures the Redis limiter.
type Config struct {
	Addr      string
	Password  string
	DB        int
	Rate      int           // max requests per Window
	Window    time.Duration // fixed window length
	KeyPrefix string        // default "ratelimit:"
}

// RedisLimiter is a distributed fixed-window RateLimiter backed by Redis.
type RedisLimiter struct {
	client    *redis.Client
	rate      int
	window    time.Duration
	keyPrefix string
}

// Ensure interface compliance.
var _ atlas.RateLimiter = (*RedisLimiter)(nil)

// Redis constructs a RedisLimiter, dialing its own client from cfg.
func Redis(cfg Config) (*RedisLimiter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("ratelimit: redis ping: %w", err)
	}
	return NewWithClient(client, cfg.Rate, cfg.Window, cfg.KeyPrefix)
}

// NewWithClient constructs a RedisLimiter using an existing client (e.g. the
// one from the cache pillar), so the connection pool is shared.
func NewWithClient(client *redis.Client, rate int, window time.Duration, keyPrefix string) (*RedisLimiter, error) {
	if client == nil {
		return nil, fmt.Errorf("ratelimit: nil redis client")
	}
	if rate <= 0 {
		return nil, fmt.Errorf("ratelimit: rate must be > 0")
	}
	if window <= 0 {
		return nil, fmt.Errorf("ratelimit: window must be > 0")
	}
	if keyPrefix == "" {
		keyPrefix = "ratelimit:"
	}
	return &RedisLimiter{client: client, rate: rate, window: window, keyPrefix: keyPrefix}, nil
}

// Allow reports whether the request for key may proceed under the fixed-window
// limit. A Redis error is returned to the caller (the middleware fails open).
func (l *RedisLimiter) Allow(ctx context.Context, key string) (bool, error) {
	ms := fmt.Sprintf("%d", l.window.Milliseconds())
	res, err := fixedWindow.Run(ctx, l.client,
		[]string{l.keyPrefix + key}, ms, l.rate).Int64()
	if err != nil {
		return false, fmt.Errorf("ratelimit: %w", err)
	}
	return res == 1, nil
}
