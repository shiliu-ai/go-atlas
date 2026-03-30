package tracing

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/shiliu-ai/go-atlas/atlas"
)

// Tracer is the tracing Pillar wrapper.
type Tracer struct {
	shutdown func(context.Context) error
	cfg      Config
}

// Pillar returns an atlas.Option that registers the tracing Pillar.
func Pillar(opts ...Option) atlas.Option {
	return func(a *atlas.Atlas) {
		t := &Tracer{}
		for _, opt := range opts {
			opt(t)
		}
		a.Register(t)
	}
}

// Of retrieves the Tracer from an Atlas instance.
func Of(a *atlas.Atlas) *Tracer {
	return atlas.Use[*Tracer](a)
}

// Option configures the tracing Pillar.
type Option func(*Tracer)

// Ensure interface compliance.
var _ atlas.Pillar = (*Tracer)(nil)
var _ atlas.MiddlewareProvider = (*Tracer)(nil)

func (t *Tracer) Name() string { return "tracing" }

func (t *Tracer) Init(core *atlas.Core) error {
	var cfg Config
	if err := core.Unmarshal("tracing", &cfg); err != nil {
		return fmt.Errorf("tracing: %w", err)
	}
	t.cfg = cfg

	shutdown, err := initTracing(context.Background(), cfg)
	if err != nil {
		return err
	}
	t.shutdown = shutdown
	return nil
}

func (t *Tracer) Stop(ctx context.Context) error {
	if t.shutdown != nil {
		return t.shutdown(ctx)
	}
	return nil
}

func (t *Tracer) Middleware() []gin.HandlerFunc {
	serviceName := t.cfg.ServiceName
	if serviceName == "" {
		serviceName = "atlas"
	}
	return []gin.HandlerFunc{otelgin.Middleware(serviceName)}
}
