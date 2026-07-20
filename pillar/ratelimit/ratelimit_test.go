package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

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

func TestNewWithClient_CloseDoesNotCloseSharedClient(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	l, err := NewWithClient(client, 10, time.Minute, "")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Close must be a no-op for a shared (injected) client.
	if err := l.Close(); err != nil {
		t.Fatalf("close shared client limiter: %v", err)
	}
	// The shared client must still be usable (not closed): a command returns a
	// connection error (no server) rather than redis.ErrClosed.
	if err := client.Ping(context.Background()).Err(); err == redis.ErrClosed {
		t.Fatal("shared client was closed by limiter.Close()")
	}
	_ = client.Close()
}
