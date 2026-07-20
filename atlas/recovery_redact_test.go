package atlas

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRedactedRequestDump verifies sensitive headers are redacted from the
// panic request dump, non-sensitive headers survive, and the original request
// is not mutated.
func TestRedactedRequestDump(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer supersecret")
	req.Header.Set("Cookie", "session=abc123")
	req.Header.Set("X-Trace-Id", "keepme")

	dump := redactedRequestDump(req)

	if strings.Contains(dump, "supersecret") || strings.Contains(dump, "session=abc123") {
		t.Fatalf("credential leaked into dump: %q", dump)
	}
	if !strings.Contains(dump, "[REDACTED]") {
		t.Fatalf("expected redaction marker in dump: %q", dump)
	}
	if !strings.Contains(dump, "keepme") {
		t.Fatalf("non-sensitive header should be kept: %q", dump)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer supersecret" {
		t.Fatalf("original request header was mutated: %q", got)
	}
}

func TestRedactedRequestDump_NilRequest(t *testing.T) {
	if dump := redactedRequestDump(nil); dump != "" {
		t.Fatalf("nil request should yield empty dump, got %q", dump)
	}
}

// TestRedactedRequestDump_QueryParams verifies credentials passed as query
// parameters are redacted, ordinary parameters survive, and the original
// request URL is not mutated.
func TestRedactedRequestDump_QueryParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/cb?code=authsecret&access_token=tokensecret&page=2", nil)

	dump := redactedRequestDump(req)

	if strings.Contains(dump, "authsecret") || strings.Contains(dump, "tokensecret") {
		t.Fatalf("query credential leaked into dump: %q", dump)
	}
	// The marker may be percent-encoded (%5BREDACTED%5D) in the query string.
	if !strings.Contains(dump, "REDACTED") {
		t.Fatalf("expected redaction marker in dump: %q", dump)
	}
	if !strings.Contains(dump, "page=2") {
		t.Fatalf("non-sensitive query param should be kept: %q", dump)
	}
	if got := req.URL.RawQuery; got != "code=authsecret&access_token=tokensecret&page=2" {
		t.Fatalf("original request URL was mutated: %q", got)
	}
}
