package cache

import (
	"context"
	"time"
)

// Lock is the distributed lock interface.
type Lock interface {
	// Acquire attempts to acquire the lock. Returns true if successful.
	Acquire(ctx context.Context) (bool, error)
	// Release releases the lock. Only the holder should release.
	Release(ctx context.Context) error
	// Extend extends the lock TTL. Useful for long-running tasks.
	Extend(ctx context.Context, ttl time.Duration) (bool, error)
}
