package ratelimit

import (
	"testing"
	"time"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Compile-time check that RedisLimiter satisfies atlas.RateLimiter.
var _ atlas.RateLimiter = (*RedisLimiter)(nil)

func TestRedis_UnreachableErrors(t *testing.T) {
	// Port 1 is not listenable; Ping should fail fast.
	if _, err := Redis(Config{Addr: "127.0.0.1:1", Rate: 10, Window: time.Minute}); err == nil {
		t.Fatal("expected error constructing limiter against unreachable redis")
	}
}

func TestRedis_InvalidConfig(t *testing.T) {
	if _, err := Redis(Config{Addr: "127.0.0.1:6379", Rate: 0, Window: time.Minute}); err == nil {
		t.Fatal("expected error for non-positive rate")
	}
}
