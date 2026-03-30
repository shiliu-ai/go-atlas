package serviceclient

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"

	atlas "github.com/shiliu-ai/go-atlas"
	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/pillar/httpclient"
)

// Pillar returns an atlas.Option that registers the serviceclient Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		m := &Manager{}
		for _, opt := range opts {
			opt(m)
		}
		a.Register(m)
	}
}

// Of retrieves the serviceclient Manager from an Atlas instance.
func Of(a *atlas.Atlas) *Manager {
	return atlas.Use[*Manager](a)
}

// Option configures the serviceclient Pillar.
type Option func(*Manager)

// Ensure interface compliance.
var _ atlas.Pillar = (*Manager)(nil)
var _ atlas.MiddlewareProvider = (*Manager)(nil)

func (m *Manager) Name() string { return "services" }

func (m *Manager) Init(core *atlas.Core) error {
	var services map[string]ServiceConfig
	if err := core.Unmarshal("services", &services); err != nil {
		return fmt.Errorf("services: %w", err)
	}

	// Read httpclient defaults for fallback.
	var defaults httpclient.Config
	_ = core.Unmarshal("httpclient", &defaults)

	logger := core.Logger("serviceclient")
	if logger == nil {
		logger = log.Global()
	}

	m.clients = make(map[string]*Client, len(services))
	m.defaults = defaults
	m.logger = logger

	for name, cfg := range services {
		m.clients[name] = newClient(name, cfg, defaults, logger)
	}
	return nil
}

func (m *Manager) Stop(_ context.Context) error {
	return nil
}

// Middleware returns the ForwardHeaders middleware for automatic header propagation.
func (m *Manager) Middleware() []gin.HandlerFunc {
	return []gin.HandlerFunc{ForwardHeaders()}
}
