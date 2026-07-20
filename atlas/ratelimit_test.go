package atlas

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

type fakeLimiter struct {
	allow bool
	err   error
}

func (f fakeLimiter) Allow(context.Context, string) (bool, error) { return f.allow, f.err }

func TestRateLimitMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		limiter    fakeLimiter
		wantStatus int
	}{
		{"allow passes through", fakeLimiter{allow: true}, http.StatusOK},
		{"deny returns 429", fakeLimiter{allow: false}, http.StatusTooManyRequests},
		{"backend error fails open", fakeLimiter{err: errors.New("backend down")}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(rateLimitMiddleware(tc.limiter, log.NewDefault(log.LevelError)))
			r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			r.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestMemStore_Allow(t *testing.T) {
	s := &memStore{
		buckets: make(map[string]*rlBucket),
		rate:    2,
		window:  time.Minute,
		done:    make(chan struct{}),
	}
	var _ RateLimiter = s

	allow := func() bool {
		ok, err := s.Allow(context.Background(), "k")
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !allow() {
		t.Fatal("1st request should be allowed")
	}
	if !allow() {
		t.Fatal("2nd request should be allowed")
	}
	if allow() {
		t.Fatal("3rd request should be denied within the window")
	}
}

// TestMemStore_StopIdempotent verifies stop() can be called more than once
// without panicking on a double channel close.
func TestMemStore_StopIdempotent(t *testing.T) {
	s := &memStore{
		buckets: make(map[string]*rlBucket),
		rate:    1,
		window:  time.Minute,
		done:    make(chan struct{}),
	}
	s.stop()
	s.stop() // must not panic
}

// blockingLimiter blocks until its context is cancelled, to exercise the
// Allow timeout / fail-open path.
type blockingLimiter struct{}

func (blockingLimiter) Allow(ctx context.Context, _ string) (bool, error) {
	<-ctx.Done()
	return false, ctx.Err()
}

func TestRateLimitMiddleware_TimesOutFailsOpen(t *testing.T) {
	old := rateLimitTimeout
	rateLimitTimeout = 30 * time.Millisecond
	defer func() { rateLimitTimeout = old }()

	r := gin.New()
	r.Use(rateLimitMiddleware(blockingLimiter{}, log.NewDefault(log.LevelError)))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("blocked limiter should time out and fail open (200), got %d", rec.Code)
	}
}
