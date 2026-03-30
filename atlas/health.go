package atlas

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerHealthRoutes adds /healthz, /livez, and /readyz endpoints.
func (a *Atlas) registerHealthRoutes() {
	a.srv.engine.GET("/healthz", a.healthzHandler)
	a.srv.engine.GET("/livez", a.livezHandler)
	a.srv.engine.GET("/readyz", a.readyzHandler)
}

// healthzHandler returns the aggregated health status of all Pillars
// that implement HealthChecker.
func (a *Atlas) healthzHandler(c *gin.Context) {
	errs := a.checkHealth(c)
	if len(errs) > 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"errors": errs,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// livezHandler always returns 200 — indicates the process is alive.
func (a *Atlas) livezHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// readyzHandler checks all HealthChecker Pillars to determine readiness.
func (a *Atlas) readyzHandler(c *gin.Context) {
	errs := a.checkHealth(c)
	if len(errs) > 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"errors": errs,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// checkHealth iterates registered Pillars and returns error messages from any
// that implement HealthChecker and report unhealthy.
func (a *Atlas) checkHealth(c *gin.Context) map[string]string {
	errs := make(map[string]string)
	for _, p := range a.registry.Pillars() {
		if hc, ok := p.(HealthChecker); ok {
			if err := hc.Health(c.Request.Context()); err != nil {
				errs[p.Name()] = err.Error()
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
