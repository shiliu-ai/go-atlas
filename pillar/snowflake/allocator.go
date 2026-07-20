package snowflake

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// maxWorkerID is the largest Snowflake worker ID (must match artifact/id's
// 10-bit worker field: (1<<10)-1).
const maxWorkerID = 1023

// Allocator obtains and maintains a unique worker ID.
type Allocator interface {
	// Acquire obtains a unique worker ID in [0, maxWorkerID].
	Acquire(ctx context.Context) (int64, error)
	// Renew refreshes the lease; returns false if the lease is no longer held.
	Renew(ctx context.Context) (bool, error)
	// Release frees the worker ID.
	Release(ctx context.Context) error
	// Close releases any underlying resources (e.g. the Redis client).
	Close() error
}

// staticAllocator returns a fixed, configured worker ID and never expires.
type staticAllocator struct{ workerID int64 }

func (s *staticAllocator) Acquire(context.Context) (int64, error) { return s.workerID, nil }
func (s *staticAllocator) Renew(context.Context) (bool, error)    { return true, nil }
func (s *staticAllocator) Release(context.Context) error          { return nil }
func (s *staticAllocator) Close() error                           { return nil }

// owner-checked lease scripts (renew/release only if we still hold the key).
var renewScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)

// redisAllocator claims a worker ID by scanning the ID space with SET NX and
// keeps it via a TTL lease.
type redisAllocator struct {
	client    *redis.Client
	keyPrefix string
	ttl       time.Duration
	token     string
	key       string // set once acquired
}

func newRedisAllocator(cfg Config) (*redisAllocator, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("snowflake: redis ping: %w", err)
	}
	return &redisAllocator{
		client:    client,
		keyPrefix: cfg.KeyPrefix,
		ttl:       cfg.TTL,
		token:     uuid.NewString(),
	}, nil
}

func (r *redisAllocator) Acquire(ctx context.Context) (int64, error) {
	// Start at a random offset so simultaneous startups don't all contend on ID 0.
	start := int64(rand.IntN(maxWorkerID + 1))
	for n := int64(0); n <= maxWorkerID; n++ {
		i := (start + n) % (maxWorkerID + 1)
		key := fmt.Sprintf("%s%d", r.keyPrefix, i)
		ok, err := r.client.SetNX(ctx, key, r.token, r.ttl).Result()
		if err != nil {
			return 0, fmt.Errorf("snowflake: acquire worker id: %w", err)
		}
		if ok {
			r.key = key
			return i, nil
		}
	}
	return 0, fmt.Errorf("snowflake: no free worker id (max %d instances)", maxWorkerID+1)
}

func (r *redisAllocator) Renew(ctx context.Context) (bool, error) {
	ms := fmt.Sprintf("%d", r.ttl.Milliseconds())
	res, err := renewScript.Run(ctx, r.client, []string{r.key}, r.token, ms).Int64()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (r *redisAllocator) Release(ctx context.Context) error {
	if r.key == "" {
		return nil
	}
	if err := releaseScript.Run(ctx, r.client, []string{r.key}, r.token).Err(); err != nil {
		return fmt.Errorf("snowflake: release worker id: %w", err)
	}
	return nil
}

func (r *redisAllocator) Close() error { return r.client.Close() }
