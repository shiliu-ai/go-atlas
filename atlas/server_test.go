package atlas

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestServerListenServe verifies that listen() binds the socket (so a
// successful return means the server is accepting connections) and that
// serve() serves requests on the bound listener until shutdown.
func TestServerListenServe(t *testing.T) {
	s := newServer(serverConfig{Port: 0})
	// newServer defaults port 0 to 8080; bind an ephemeral port for the test.
	s.cfg.Port = 0
	s.engine.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	ln, err := s.listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = s.serve(ln) }()
	defer func() { _ = s.shutdown(context.Background()) }()

	port := ln.Addr().(*net.TCPAddr).Port
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/ping", port))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
