package atlas

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

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
				defer func() {
					if r := recover(); r != nil {
						a.logger.Error(ctx, "pillar panic recovered",
							log.F("pillar", name),
							log.F("error", r),
							log.F("stack", string(debug.Stack())),
						)
						errCh <- fmt.Errorf("pillar %s panicked: %v", name, r)
					}
				}()
				a.logger.Info(ctx, "pillar starting", log.F("pillar", name))
				if err := s.Start(ctx); err != nil {
					errCh <- err
				}
			}(s, p.Name())
		}
	}

	// Bind the listener in the foreground so readiness flips to ready only
	// after the socket is actually accepting connections.
	ln, err := a.srv.listen()
	if err != nil {
		a.logger.Error(ctx, "http server listen failed", log.F("error", err))
		return a.shutdown(context.Background())
	}

	// Serve requests in a goroutine.
	go func() {
		a.logger.Info(ctx, "http server starting",
			log.F("port", a.srv.cfg.Port),
		)
		if err := a.srv.serve(ln); err != nil {
			errCh <- err
		}
	}()

	// Listener is up and Starters launched: accept traffic.
	a.setReadiness(readinessReady)
	a.logger.Info(ctx, "atlas ready", log.F("name", a.name))

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

// shutdown performs graceful shutdown: mark the instance draining (so load
// balancers stop routing to it), stop the HTTP server, then Pillars in
// reverse registration order, then clean up internal resources.
func (a *Atlas) shutdown(ctx context.Context) error {
	a.logger.Info(ctx, "atlas shutting down", log.F("name", a.name))

	// Refuse new traffic first so load balancers drain this instance via
	// /readyz before the server stops.
	a.setReadiness(readinessDraining)

	// Give load balancers time to observe the draining state and stop routing
	// new traffic before we stop accepting. Endpoint removal propagates
	// asynchronously in Kubernetes, so this closes the black-hole window.
	if d := a.srv.cfg.PreShutdownDelay; d > 0 {
		a.logger.Info(ctx, "pre-shutdown drain delay", log.F("delay", d.String()))
		select {
		case <-time.After(d):
		case <-ctx.Done():
		}
	}

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

	// Close an injected rate limiter that owns resources (e.g. a Redis client).
	if closer, ok := a.rateLimiter.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			a.logger.Error(ctx, "rate limiter close error", log.F("error", err))
		}
	}

	a.logger.Info(ctx, "atlas stopped", log.F("name", a.name))
	return nil
}
