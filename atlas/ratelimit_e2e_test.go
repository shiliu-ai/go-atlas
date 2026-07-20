package atlas_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// denyLimiter is an atlas.RateLimiter that denies every request.
type denyLimiter struct{}

func (denyLimiter) Allow(context.Context, string) (bool, error) { return false, nil }

// TestWithRateLimiter_InjectedLimiterEnforced verifies that a limiter injected
// via WithRateLimiter is actually wired into the middleware chain: a deny-all
// limiter must turn any request into 429 through the real Atlas engine.
func TestWithRateLimiter_InjectedLimiterEnforced(t *testing.T) {
	dir := writeConfig(t, "")
	a := atlas.New("t", atlas.WithConfigPaths(dir), atlas.WithRateLimiter(denyLimiter{}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	a.Engine().ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("with an injected deny-all limiter, /livez = %d, want 429", w.Code)
	}
}
