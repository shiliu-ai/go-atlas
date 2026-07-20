package atlas

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestReadinessState_String(t *testing.T) {
	cases := map[readinessState]string{
		readinessStarting: "starting",
		readinessReady:    "ready",
		readinessDraining: "draining",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("state %d String() = %q, want %q", int32(state), got, want)
		}
	}
}

func TestReadyzHandler_ReflectsState(t *testing.T) {
	cases := []struct {
		state      readinessState
		wantStatus int
		wantBody   string
	}{
		{readinessStarting, http.StatusServiceUnavailable, "starting"},
		{readinessReady, http.StatusOK, "ready"},
		{readinessDraining, http.StatusServiceUnavailable, "draining"},
	}
	for _, tc := range cases {
		t.Run(tc.wantBody, func(t *testing.T) {
			a := &Atlas{}
			a.setReadiness(tc.state)

			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)

			a.readyzHandler(c)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body = %q, want to contain %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

// TestReadyzHandler_IgnoresDependencies guards against re-introducing shared
// dependency checks into /readyz: even with an unhealthy registered
// HealthChecker, a ready instance must report 200 (deps belong to /healthz).
func TestReadyzHandler_IgnoresDependencies(t *testing.T) {
	a := &Atlas{registry: newPillarRegistry()}
	a.registry.Register(unhealthyPillar{})
	a.setReadiness(readinessReady)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/readyz", nil)

	a.readyzHandler(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200 despite unhealthy dependency", rec.Code)
	}
}

// unhealthyPillar is a HealthChecker Pillar that always reports unhealthy.
type unhealthyPillar struct{}

func (unhealthyPillar) Name() string                 { return "always-unhealthy" }
func (unhealthyPillar) Init(*Core) error             { return nil }
func (unhealthyPillar) Stop(context.Context) error   { return nil }
func (unhealthyPillar) Health(context.Context) error { return errors.New("always unhealthy") }
