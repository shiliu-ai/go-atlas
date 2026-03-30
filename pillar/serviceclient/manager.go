package serviceclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/pillar/httpclient"
)

// Manager manages named service clients.
// It acts as a service registry, mapping service names to their HTTP clients.
type Manager struct {
	clients  map[string]*Client
	defaults httpclient.Config
	logger   log.Logger
}

// Get returns the client for the named service.
// Returns an error if the service is not configured.
func (m *Manager) Get(name string) (*Client, error) {
	c, ok := m.clients[name]
	if !ok {
		return nil, fmt.Errorf("serviceclient: service %q not configured", name)
	}
	return c, nil
}

// MustGet returns the client for the named service.
// Panics if the service is not configured.
func (m *Manager) MustGet(name string) *Client {
	c, err := m.Get(name)
	if err != nil {
		panic(err)
	}
	return c
}

// Names returns all registered service names.
func (m *Manager) Names() []string {
	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// Ping checks connectivity to the named service by sending a GET /health request.
// Returns nil if the service responds with a 2xx status code.
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.DoRaw(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return fmt.Errorf("serviceclient[%s]: ping failed: %w", c.name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("serviceclient[%s]: ping returned HTTP %d", c.name, resp.StatusCode)
	}
	return nil
}

// PingAll checks connectivity to all registered services.
// Returns the first error encountered, or nil if all services are healthy.
func (m *Manager) PingAll(ctx context.Context) error {
	for name, c := range m.clients {
		if err := c.Ping(ctx); err != nil {
			m.logger.Error(ctx, "service ping failed",
				log.F("service", name),
				log.F("error", err),
			)
			return err
		}
		m.logger.Info(ctx, "service ping ok", log.F("service", name))
	}
	return nil
}
