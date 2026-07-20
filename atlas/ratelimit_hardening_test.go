package atlas_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// TestRateLimit_WindowDefaultedNotBypassed: rate set but window omitted must
// still limit (not silently allow everything).
func TestRateLimit_WindowDefaultedNotBypassed(t *testing.T) {
	dir := writeConfig(t, "middleware:\n  rate_limit:\n    rate: 2\n")
	a := atlas.New("t", atlas.WithConfigPaths(dir))

	codes := make([]int, 4)
	for i := range codes {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/livez", nil)
		req.RemoteAddr = "203.0.113.7:5000" // stable client IP
		a.Engine().ServeHTTP(w, req)
		codes[i] = w.Code
	}
	// rate=2 within the defaulted window: first 2 pass, later ones 429.
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK {
		t.Fatalf("first 2 should pass, got %v", codes)
	}
	if codes[2] != http.StatusTooManyRequests && codes[3] != http.StatusTooManyRequests {
		t.Fatalf("window unset must NOT bypass limiting; got %v", codes)
	}
}

// nilRL is a typed-nil RateLimiter used to check WithRateLimiter doesn't panic.
type nilRL struct{}

func (*nilRL) Allow(context.Context, string) (bool, error) { return false, nil }

func TestWithRateLimiter_TypedNilIgnored(t *testing.T) {
	dir := writeConfig(t, "")
	var typed *nilRL // typed-nil
	a := atlas.New("t", atlas.WithConfigPaths(dir), atlas.WithRateLimiter(typed))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	a.Engine().ServeHTTP(w, req) // must not panic; no limiter configured
	if w.Code != http.StatusOK {
		t.Fatalf("typed-nil limiter should be ignored, got %d", w.Code)
	}
}
