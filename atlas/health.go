package atlas

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// registerHealthRoutes adds /healthz, /livez, and /readyz endpoints.
func (a *Atlas) registerHealthRoutes() {
	a.srv.engine.GET("/healthz", a.healthzHandler)
	a.srv.engine.GET("/livez", a.livezHandler)
	a.srv.engine.GET("/readyz", a.readyzHandler)
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

// readyzHandler checks all HealthChecker Pillars to determine readiness.
func (a *Atlas) readyzHandler(c *gin.Context) {
	overall, pillars := a.checkHealth(c)
	status := http.StatusOK
	statusText := "healthy"
	if overall != "healthy" {
		status = http.StatusServiceUnavailable
		statusText = "not ready"
	}
	resp := gin.H{"status": statusText}
	if len(pillars) > 0 {
		resp["pillars"] = pillars
	}
	c.JSON(status, resp)
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
