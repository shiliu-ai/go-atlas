package lock

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// release only if the value matches (owner check).
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
end
return 0
`)

// extend only if the value matches (owner check).
var extendScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

// RedisLock implements Lock using Redis SET NX.
type RedisLock struct {
	client *redis.Client
	key    string
	value  string // unique owner token
	ttl    time.Duration
}

// NewRedis creates a new Redis-based distributed lock.
// key is the lock name, ttl is the auto-expire duration to prevent deadlocks.
func NewRedis(client *redis.Client, key string, ttl time.Duration) *RedisLock {
	return &RedisLock{
		client: client,
		key:    "lock:" + key,
		value:  uuid.New().String(),
		ttl:    ttl,
	}
}

func (l *RedisLock) Acquire(ctx context.Context) (bool, error) {
	ok, err := l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("lock: acquire %s: %w", l.key, err)
	}
	return ok, nil
}

func (l *RedisLock) Release(ctx context.Context) error {
	result, err := releaseScript.Run(ctx, l.client, []string{l.key}, l.value).Int64()
	if err != nil {
		return fmt.Errorf("lock: release %s: %w", l.key, err)
	}
	if result == 0 {
		return fmt.Errorf("lock: release %s: not the lock owner", l.key)
	}
	return nil
}

func (l *RedisLock) Extend(ctx context.Context, ttl time.Duration) (bool, error) {
	ms := fmt.Sprintf("%d", ttl.Milliseconds())
	result, err := extendScript.Run(ctx, l.client, []string{l.key}, l.value, ms).Int64()
	if err != nil {
		return false, fmt.Errorf("lock: extend %s: %w", l.key, err)
	}
	return result == 1, nil
}

// AcquireWithRetry attempts to acquire the lock with retries.
func (l *RedisLock) AcquireWithRetry(ctx context.Context, retries int, retryInterval time.Duration) (bool, error) {
	for i := range retries {
		ok, err := l.Acquire(ctx)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		if i < retries-1 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(retryInterval):
			}
		}
	}
	return false, nil
}
