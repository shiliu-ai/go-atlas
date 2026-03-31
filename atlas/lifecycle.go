package atlas

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/shiliu-ai/go-atlas/aether/log"
)

// run starts all Pillar Starters, the HTTP server, and waits for a
// termination signal before performing graceful shutdown.
func (a *Atlas) run(ctx context.Context) error {
	a.logger.Info(ctx, "atlas starting", log.F("name", a.name))

	// Start Pillars that implement Starter.
	errCh := make(chan error, len(a.registry.Pillars())+1)
	for _, p := range a.registry.Pillars() {
		if s, ok := p.(Starter); ok {
			go func(s Starter, name string) {
				a.logger.Info(ctx, "pillar starting", log.F("pillar", name))
				if err := s.Start(ctx); err != nil {
					errCh <- err
				}
			}(s, p.Name())
		}
	}

	// Start HTTP server in a goroutine.
	go func() {
		a.logger.Info(ctx, "http server starting",
			log.F("port", a.srv.cfg.Port),
		)
		if err := a.srv.start(); err != nil {
			errCh <- err
		}
	}()

	// Wait for signal or error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		a.logger.Info(ctx, "received signal", log.F("signal", sig.String()))
	case err := <-errCh:
		a.logger.Error(ctx, "component error", log.F("error", err))
	case <-ctx.Done():
		a.logger.Info(ctx, "context cancelled")
	}

	return a.shutdown(context.Background())
}

// shutdown performs graceful shutdown: stop server first, then Pillars
// in reverse registration order, then clean up internal resources.
func (a *Atlas) shutdown(ctx context.Context) error {
	a.logger.Info(ctx, "atlas shutting down", log.F("name", a.name))

	// Stop HTTP server first.
	if err := a.srv.shutdown(ctx); err != nil {
		a.logger.Error(ctx, "server shutdown error", log.F("error", err))
	}

	// Stop Pillars in reverse order.
	pillars := a.registry.Pillars()
	for i := len(pillars) - 1; i >= 0; i-- {
		p := pillars[i]
		a.logger.Info(ctx, "stopping pillar", log.F("pillar", p.Name()))
		if err := p.Stop(ctx); err != nil {
			a.logger.Error(ctx, "pillar stop error",
				log.F("pillar", p.Name()),
				log.F("error", err),
			)
		}
	}

	// Stop rate limiter cleanup goroutine if active.
	if a.rateLimitStore != nil {
		a.rateLimitStore.stop()
	}

	a.logger.Info(ctx, "atlas stopped", log.F("name", a.name))
	return nil
}
