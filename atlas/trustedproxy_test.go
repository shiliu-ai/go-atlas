package atlas

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestServer_TrustedProxies verifies that ClientIP() ignores a spoofed
// X-Forwarded-For by default (no trusted proxy), and honors it only for
// configured trusted proxies.
func TestServer_TrustedProxies(t *testing.T) {
	clientIP := func(s *server) string {
		s.engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = "10.1.2.3:9999"
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		s.engine.ServeHTTP(rec, req)
		return rec.Body.String()
	}

	t.Run("default trusts no proxy: XFF ignored", func(t *testing.T) {
		if got := clientIP(newServer(serverConfig{})); got != "10.1.2.3" {
			t.Fatalf("ClientIP = %q, want socket peer 10.1.2.3 (X-Forwarded-For must be ignored)", got)
		}
	})

	t.Run("configured trusted proxy: XFF honored", func(t *testing.T) {
		s := newServer(serverConfig{TrustedProxies: []string{"10.1.2.3/32"}})
		if got := clientIP(s); got != "1.2.3.4" {
			t.Fatalf("ClientIP = %q, want forwarded 1.2.3.4", got)
		}
	})
}
