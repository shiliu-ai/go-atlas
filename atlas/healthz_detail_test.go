package atlas_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// TestHealthz_DetailGatedByConfig verifies /healthz hides the per-pillar
// breakdown by default (no backend enumeration) and includes it only when
// health.show_details is enabled.
func TestHealthz_DetailGatedByConfig(t *testing.T) {
	get := func(a *atlas.Atlas) string {
		w := httptest.NewRecorder()
		a.Engine().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		return w.Body.String()
	}

	t.Run("default hides pillar detail", func(t *testing.T) {
		dir := writeConfig(t, "")
		a := atlas.New("t", atlas.WithConfigPaths(dir), pillarOpt(&healthPillar{name: "db", healthy: true}))
		body := get(a)
		if strings.Contains(body, "db") {
			t.Fatalf("/healthz leaked pillar name by default: %s", body)
		}
		if !strings.Contains(body, "healthy") {
			t.Fatalf("/healthz should still report aggregate status: %s", body)
		}
	})

	t.Run("show_details exposes pillar detail", func(t *testing.T) {
		dir := writeConfig(t, "health:\n  show_details: true\n")
		a := atlas.New("t", atlas.WithConfigPaths(dir), pillarOpt(&healthPillar{name: "db", healthy: true}))
		body := get(a)
		if !strings.Contains(body, "db") {
			t.Fatalf("/healthz with show_details should include pillar name: %s", body)
		}
	})
}
