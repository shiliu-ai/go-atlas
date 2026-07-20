package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

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

func newTestReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
	return reader
}

func sumCounter(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, dp := range sum.DataPoints {
					total += dp.Value
				}
			}
		}
	}
	return total
}

func TestRateLimit_Metrics(t *testing.T) {
	reader := newTestReader(t)
	l := &RedisLimiter{keyPrefix: "ratelimit:"} // no client needed for record()

	l.record("allowed")
	l.record("allowed")
	l.record("denied")

	if got := sumCounter(t, reader, "ratelimit.decisions"); got != 3 {
		t.Fatalf("ratelimit.decisions = %d, want 3", got)
	}
}
