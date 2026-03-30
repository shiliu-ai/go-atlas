package cache

import (
	"context"
	"fmt"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Pillar returns an atlas.Option that registers the cache Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		c := &RedisCache{}
		for _, opt := range opts {
			opt(c)
		}
		a.Register(c)
	}
}

// Of retrieves the RedisCache from an Atlas instance.
func Of(a *atlas.Atlas) *RedisCache {
	return atlas.Use[*RedisCache](a)
}

// Option configures the cache Pillar.
type Option func(*RedisCache)

// Ensure interface compliance.
var _ atlas.Pillar = (*RedisCache)(nil)
var _ atlas.HealthChecker = (*RedisCache)(nil)

func (r *RedisCache) Name() string { return "redis" }

func (r *RedisCache) Init(core *atlas.Core) error {
	var cfg RedisConfig
	if err := core.Unmarshal("redis", &cfg); err != nil {
		return fmt.Errorf("redis: %w", err)
	}

	rc, err := newRedis(cfg)
	if err != nil {
		return err
	}
	r.client = rc.client
	return nil
}

func (r *RedisCache) Stop(_ context.Context) error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

func (r *RedisCache) Health(ctx context.Context) error {
	if r.client == nil {
		return fmt.Errorf("redis: not initialized")
	}
	return r.client.Ping(ctx).Err()
}
