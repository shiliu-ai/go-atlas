package serviceclient

import (
	"fmt"

	"github.com/shiliu-ai/go-atlas/httpclient"
	"github.com/shiliu-ai/go-atlas/log"
)

// Manager manages named service clients.
// It acts as a service registry, mapping service names to their HTTP clients.
type Manager struct {
	clients  map[string]*Client
	defaults httpclient.Config
	logger   log.Logger
}

// NewManager creates a Manager from service configurations.
// The defaults parameter provides fallback values for timeout/retry
// when a service doesn't specify its own.
func NewManager(services map[string]ServiceConfig, defaults httpclient.Config, logger log.Logger) *Manager {
	if logger == nil {
		logger = log.Global()
	}
	m := &Manager{
		clients:  make(map[string]*Client, len(services)),
		defaults: defaults,
		logger:   logger,
	}
	for name, cfg := range services {
		m.clients[name] = newClient(name, cfg, defaults, logger)
	}
	return m
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
