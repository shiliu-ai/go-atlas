package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/shiliu-ai/go-atlas/log"
)

// App is the application lifecycle manager with dependency injection container.
type App struct {
	name       string
	logger     log.Logger
	components []Component
	shutdowns  []func(context.Context) error
	mu         sync.Mutex
}

// Component is an interface for application components that have a lifecycle.
type Component interface {
	Name() string
}

// Starter is a component that can be started.
type Starter interface {
	Start() error
}

// Stopper is a component that can be stopped.
type Stopper interface {
	Stop(ctx context.Context) error
}

// New creates a new App.
func New(name string, logger log.Logger) *App {
	if logger == nil {
		logger = log.NewDefault(log.LevelInfo)
	}
	return &App{
		name:   name,
		logger: logger,
	}
}

// Register adds a component to the app.
func (a *App) Register(c Component) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.components = append(a.components, c)
}

// OnShutdown registers a shutdown hook.
func (a *App) OnShutdown(fn func(context.Context) error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.shutdowns = append(a.shutdowns, fn)
}

// Run starts all Starter components and waits for OS signals to gracefully shut down.
func (a *App) Run(ctx context.Context) error {
	a.logger.Info(ctx, "app starting", log.F("name", a.name))

	// Start all Starter components in goroutines.
	errCh := make(chan error, len(a.components))
	for _, c := range a.components {
		if s, ok := c.(Starter); ok {
			go func(s Starter, name string) {
				a.logger.Info(ctx, "component starting", log.F("component", name))
				if err := s.Start(); err != nil {
					errCh <- err
				}
			}(s, c.Name())
		}
	}

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

	return a.shutdown(ctx)
}

func (a *App) shutdown(ctx context.Context) error {
	a.logger.Info(ctx, "app shutting down", log.F("name", a.name))

	// Stop components in reverse order.
	for i := len(a.components) - 1; i >= 0; i-- {
		if s, ok := a.components[i].(Stopper); ok {
			a.logger.Info(ctx, "stopping component", log.F("component", a.components[i].Name()))
			if err := s.Stop(ctx); err != nil {
				a.logger.Error(ctx, "component stop error",
					log.F("component", a.components[i].Name()),
					log.F("error", err),
				)
			}
		}
	}

	// Run shutdown hooks in reverse order.
	for i := len(a.shutdowns) - 1; i >= 0; i-- {
		if err := a.shutdowns[i](ctx); err != nil {
			a.logger.Error(ctx, "shutdown hook error", log.F("error", err))
		}
	}

	a.logger.Info(ctx, "app stopped", log.F("name", a.name))
	return nil
}
