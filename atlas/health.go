package atlas

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// registerHealthRoutes adds /healthz, /livez, and /readyz under the
// service base group. When server.Name is empty the routes sit at the
// engine root; when set (e.g. "account") they sit at "/account/…", which
// lines up with path-prefix ingresses/gateways that forward "/account/*"
// to this service. Point k8s probes at the same path the service actually
// serves — probes hit pod:port directly, so the prefixed path works the
// same as root.
func (a *Atlas) registerHealthRoutes() {
	g := a.srv.group()
	g.GET("/healthz", a.healthzHandler)
	g.GET("/livez", a.livezHandler)
	g.GET("/readyz", a.readyzHandler)
}

// pillarStatus holds the health status and latency for a single pillar.
type pillarStatus struct {
	Status  string `json:"status"`
	Latency string `json:"latency"`
}

// healthzHandler returns the aggregated health status of all Pillars
// that implement HealthChecker.
func (a *Atlas) healthzHandler(c *gin.Context) {
	overall, pillars := a.checkHealth(c)
	status := http.StatusOK
	if overall != "healthy" {
		status = http.StatusServiceUnavailable
	}
	resp := gin.H{"status": overall}
	if len(pillars) > 0 {
		resp["pillars"] = pillars
	}
	c.JSON(status, resp)
}

// livezHandler always returns 200 — indicates the process is alive.
func (a *Atlas) livezHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

// readyzHandler reports whether this instance should receive traffic, based
// solely on its lifecycle readiness state (starting/ready/draining). It does
// NOT aggregate shared external dependencies — that is /healthz — so that a
// blip in a shared dependency does not drain every replica at once.
func (a *Atlas) readyzHandler(c *gin.Context) {
	state := a.readinessValue()
	if state == readinessReady {
		c.JSON(http.StatusOK, gin.H{"status": state.String()})
		return
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"status": state.String()})
}

// checkHealth iterates registered Pillars and returns overall status plus
// per-pillar status with latency for all HealthChecker pillars.
func (a *Atlas) checkHealth(c *gin.Context) (string, map[string]pillarStatus) {
	pillars := make(map[string]pillarStatus)
	overall := "healthy"
	for _, p := range a.registry.Pillars() {
		if hc, ok := p.(HealthChecker); ok {
			start := time.Now()
			err := hc.Health(c.Request.Context())
			latency := time.Since(start)
			ps := pillarStatus{
				Status:  "healthy",
				Latency: latency.String(),
			}
			if err != nil {
				ps.Status = "unhealthy"
				overall = "unhealthy"
			}
			pillars[p.Name()] = ps
		}
	}
	if len(pillars) == 0 {
		return overall, nil
	}
	return overall, pillars
}
