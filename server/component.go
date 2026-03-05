package server

import "context"

// Component wraps Server to satisfy app.Component, app.Starter, app.Stopper.
type Component struct {
	*Server
}

func (c *Component) Name() string              { return "http-server" }
func (c *Component) Start() error              { return c.Server.Start() }
func (c *Component) Stop(ctx context.Context) error { return c.Server.Shutdown(ctx) }
