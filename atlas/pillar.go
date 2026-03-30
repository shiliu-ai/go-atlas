package atlas

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
)

// Pillar is the unified protocol for all infrastructure components.
// Every module (database, cache, SMS, storage, etc.) implements this interface
// to participate in the Atlas lifecycle.
type Pillar interface {
	Name() string
	Init(core *Core) error
	Stop(ctx context.Context) error
}

// Starter is implemented by Pillars that need background goroutines.
type Starter interface {
	Start(ctx context.Context) error
}

// HealthChecker is implemented by Pillars that support health checks.
type HealthChecker interface {
	Health(ctx context.Context) error
}

// MiddlewareProvider is implemented by Pillars that inject global middleware.
type MiddlewareProvider interface {
	Middleware() []gin.HandlerFunc
}

// PillarRegistry holds registered Pillar instances and tracks their order.
// It will be embedded into Atlas in a future refactor.
type PillarRegistry struct {
	pillars     map[string]Pillar
	pillarOrder []string
}

// NewPillarRegistry creates an empty PillarRegistry.
func NewPillarRegistry() *PillarRegistry {
	return &PillarRegistry{
		pillars:     make(map[string]Pillar),
		pillarOrder: []string{},
	}
}

// Register adds a Pillar and tracks registration order.
// Panics if a Pillar with the same Name() is already registered.
func (r *PillarRegistry) Register(p Pillar) {
	name := p.Name()
	if _, exists := r.pillars[name]; exists {
		panic(fmt.Sprintf("atlas: duplicate pillar name %q", name))
	}
	r.pillars[name] = p
	r.pillarOrder = append(r.pillarOrder, name)
}

// Pillars returns the registered pillars in registration order.
func (r *PillarRegistry) Pillars() []Pillar {
	result := make([]Pillar, 0, len(r.pillarOrder))
	for _, name := range r.pillarOrder {
		result = append(result, r.pillars[name])
	}
	return result
}

// UsePillar retrieves a registered Pillar by concrete type.
// Panics if no Pillar of the given type is found.
func UsePillar[T Pillar](r *PillarRegistry) T {
	for _, p := range r.pillars {
		if t, ok := p.(T); ok {
			return t
		}
	}
	var zero T
	panic(fmt.Sprintf("atlas: pillar %T not registered", zero))
}

// TryUsePillar retrieves a registered Pillar by concrete type without panicking.
// Returns the pillar and true if found, zero value and false otherwise.
func TryUsePillar[T Pillar](r *PillarRegistry) (T, bool) {
	for _, p := range r.pillars {
		if t, ok := p.(T); ok {
			return t, true
		}
	}
	var zero T
	return zero, false
}
