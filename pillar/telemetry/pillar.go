// Package telemetry is the unified OpenTelemetry Pillar for Atlas.
//
// It wires a single Resource, one OTLP connection, a TracerProvider and a
// MeterProvider into the Atlas lifecycle. A single gin middleware (backed by
// otelgin) produces both HTTP spans and semconv-compliant HTTP server
// metrics, with exemplars tying histogram buckets back to trace IDs.
//
// Register once:
//
//	a := atlas.New("my-service", telemetry.Pillar())
//
// Retrieve tracer/meter wherever needed:
//
//	t := telemetry.Of(a)
//	tracer := t.Tracer("billing")
//	meter  := t.Meter("billing")
//
// When telemetry.enabled is false (or absent) all providers are NoOps, so
// downstream code never has to nil-check.
//
// Functional Options let callers extend the pipeline without touching YAML:
// inject Views, extra Resource attributes, custom SpanProcessors, a shared
// Prometheus registry, or disable global provider registration. See options.go.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/shiliu-ai/go-atlas/aether/log"
	"github.com/shiliu-ai/go-atlas/atlas"
)

// cleanupTimeout caps the time spent unwinding partially-initialised
// providers when Init fails mid-way. Bounded so a stuck exporter cannot
// block application startup indefinitely.
const cleanupTimeout = 5 * time.Second

// Telemetry is the initialized Pillar instance exposed by Of(a).
type Telemetry struct {
	opts options

	serviceName string
	cfg         Config
	logger      log.Logger

	tp trace.TracerProvider
	mp metric.MeterProvider

	// shutdowns are called in parallel during Stop.
	shutdowns []func(context.Context) error

	// promHandler is non-nil when the Prometheus pull exporter is active.
	promHandler http.Handler
}

// Pillar registers the telemetry Pillar with Atlas. Options extend the
// pipeline without touching YAML config; see WithView, WithResourceAttributes,
// WithSpanProcessor, WithPrometheusRegistry, WithSetGlobals, WithRuntimeStrict.
func Pillar(opts ...Option) atlas.Option {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return func(a *atlas.Atlas) { a.Register(&Telemetry{opts: o}) }
}

// Of retrieves the Telemetry instance from an Atlas instance.
func Of(a *atlas.Atlas) *Telemetry { return atlas.Use[*Telemetry](a) }

var (
	_ atlas.Pillar             = (*Telemetry)(nil)
	_ atlas.MiddlewareProvider = (*Telemetry)(nil)
	_ atlas.RouteProvider      = (*Telemetry)(nil)
)

// Name returns the pillar name used in the config section and logs.
func (t *Telemetry) Name() string { return "telemetry" }

// Init reads the telemetry config, builds providers, and — when enabled and
// WithSetGlobals is on — registers them as OTel globals. A disabled
// configuration installs NoOp providers. A partial failure (traces up,
// metrics down, or vice versa) unwinds cleanly so the caller sees a single
// error and no leaked exporter goroutines.
func (t *Telemetry) Init(core *atlas.Core) (err error) {
	t.serviceName = core.ServiceName()
	t.logger = core.Logger(t.Name())

	// Start from a safe baseline so that partial Init failure still leaves
	// the Pillar callable (Tracer/Meter return noop).
	t.tp = noopTracerProvider
	t.mp = noopMeterProvider

	var cfg Config
	// "section missing" is fine — a zero Config runs in NoOp mode, matching
	// the pattern used by pillar/httpclient.
	_ = core.Unmarshal("telemetry", &cfg)
	cfg.applyDefaults()
	if verr := cfg.Validate(); verr != nil {
		return verr
	}
	t.cfg = cfg

	if !cfg.Enabled {
		return nil
	}

	// Any error past this point must unwind already-installed providers.
	defer func() {
		if err != nil {
			t.cleanupAfterInitFailure()
		}
	}()

	res, err := buildResource(t.serviceName, cfg.Resource, t.opts.resourceAttrs)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err = t.initTraces(ctx, res); err != nil {
		return err
	}
	if err = t.initMetrics(ctx, res); err != nil {
		return err
	}

	if t.opts.setGlobals {
		// Only overwrite globals for signals that are actually producing
		// data. Noop-overwrite-of-a-real-global is a sharp corner when Atlas
		// shares a process with another OTel-aware framework.
		if t.tp != noopTracerProvider {
			otel.SetTracerProvider(t.tp)
		}
		if t.mp != noopMeterProvider {
			otel.SetMeterProvider(t.mp)
		}
		if t.tp != noopTracerProvider || t.mp != noopMeterProvider {
			otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			))
			// Route OTel internal errors through aether/log so export
			// failures surface in the standard log stream.
			otel.SetErrorHandler(newOTelErrorHandler(t.logger))
		}
	}

	if cfg.Metrics.Enabled && cfg.Metrics.Runtime {
		if rerr := startRuntimeMetrics(t.mp); rerr != nil {
			if t.opts.runtimeStrict {
				return rerr
			}
			t.logger.Warn(context.Background(), "runtime metrics disabled", log.F("error", rerr.Error()))
		}
	}

	return nil
}

// Stop flushes pending telemetry in parallel. Errors are joined so one slow
// or failing exporter does not serialise or mask the others.
func (t *Telemetry) Stop(ctx context.Context) error {
	if len(t.shutdowns) == 0 {
		return nil
	}
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, fn := range t.shutdowns {
		wg.Add(1)
		go func(fn func(context.Context) error) {
			defer wg.Done()
			if err := fn(ctx); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(fn)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Middleware contributes the HTTP RED middleware to the Atlas chain.
// Installed only when telemetry is enabled and at least one signal is on —
// otelgin still runs propagators and allocates per-request even with both
// signals off, so skip it when neither metrics.http nor traces are active.
func (t *Telemetry) Middleware() []gin.HandlerFunc {
	if !t.cfg.Enabled {
		return nil
	}
	httpMetricsOn := t.cfg.Metrics.Enabled && t.cfg.Metrics.HTTP
	tracesOn := t.cfg.Traces.Enabled
	if httpMetricsOn || tracesOn {
		return []gin.HandlerFunc{t.httpMiddleware()}
	}
	return nil
}

// Routes mounts the Prometheus scrape endpoint when configured.
func (t *Telemetry) Routes(group *gin.RouterGroup) {
	if t.promHandler == nil {
		return
	}
	group.GET(t.cfg.Prometheus.Path, gin.WrapH(t.promHandler))
}

// Tracer returns a named tracer from the configured TracerProvider.
//
// Construct Telemetry through Pillar(); a manually-zeroed &Telemetry{} would
// hold a nil provider, so Tracer falls back to the NoOp provider in that
// case rather than panicking — safer default for a type that can end up in
// DI containers or test fakes.
func (t *Telemetry) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
	if t.tp == nil {
		return noopTracerProvider.Tracer(name, opts...)
	}
	return t.tp.Tracer(name, opts...)
}

// Meter returns a named meter from the configured MeterProvider. See Tracer
// for the rationale on the nil fallback.
func (t *Telemetry) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	if t.mp == nil {
		return noopMeterProvider.Meter(name, opts...)
	}
	return t.mp.Meter(name, opts...)
}

// TracerProvider exposes the underlying provider for third-party instrumentation.
// Returns the NoOp provider when Telemetry has not been initialised.
func (t *Telemetry) TracerProvider() trace.TracerProvider {
	if t.tp == nil {
		return noopTracerProvider
	}
	return t.tp
}

// MeterProvider exposes the underlying provider for third-party instrumentation.
// Returns the NoOp provider when Telemetry has not been initialised.
func (t *Telemetry) MeterProvider() metric.MeterProvider {
	if t.mp == nil {
		return noopMeterProvider
	}
	return t.mp
}

// PrometheusHandler returns the Prometheus scrape handler, or nil when the
// pull exporter is disabled.
func (t *Telemetry) PrometheusHandler() http.Handler { return t.promHandler }

// --- Internal init helpers ---

func (t *Telemetry) initTraces(ctx context.Context, res *resource.Resource) error {
	if !t.cfg.Traces.Enabled || !t.cfg.OTLP.Enabled {
		// Leave t.tp as the NoOp baseline set in Init.
		return nil
	}

	exp, err := newTraceExporter(ctx, t.cfg.OTLP)
	if err != nil {
		return fmt.Errorf("telemetry: trace exporter: %w", err)
	}

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(t.cfg.Traces.sampleRate()),
		)),
	}
	for _, sp := range t.opts.spanProcessors {
		tpOpts = append(tpOpts, sdktrace.WithSpanProcessor(sp))
	}

	tp := sdktrace.NewTracerProvider(tpOpts...)
	t.tp = tp
	t.shutdowns = append(t.shutdowns, tp.Shutdown)
	return nil
}

func (t *Telemetry) initMetrics(ctx context.Context, res *resource.Resource) error {
	if !t.cfg.Metrics.Enabled {
		// Leave t.mp as the NoOp baseline set in Init.
		return nil
	}

	var readers []sdkmetric.Reader

	if t.cfg.OTLP.Enabled {
		exp, err := newMetricExporter(ctx, t.cfg.OTLP)
		if err != nil {
			return fmt.Errorf("telemetry: metric exporter: %w", err)
		}
		readers = append(readers, sdkmetric.NewPeriodicReader(exp))
	}

	if t.cfg.Prometheus.Enabled {
		reg := t.opts.promRegistry
		if reg == nil {
			reg = promclient.NewRegistry()
		}
		promReader, err := prometheus.New(prometheus.WithRegisterer(reg))
		if err != nil {
			return fmt.Errorf("telemetry: prometheus exporter: %w", err)
		}
		readers = append(readers, promReader)
		t.promHandler = promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
	}

	mOpts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithView(httpServerDurationView()),
		sdkmetric.WithCardinalityLimit(t.cfg.Metrics.cardinalityLimit()),
	}
	for _, v := range t.opts.views {
		mOpts = append(mOpts, sdkmetric.WithView(v))
	}
	for _, r := range readers {
		mOpts = append(mOpts, sdkmetric.WithReader(r))
	}
	if t.cfg.Metrics.Exemplars {
		mOpts = append(mOpts, sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter))
	} else {
		mOpts = append(mOpts, sdkmetric.WithExemplarFilter(exemplar.AlwaysOffFilter))
	}

	mp := sdkmetric.NewMeterProvider(mOpts...)
	t.mp = mp
	t.shutdowns = append(t.shutdowns, mp.Shutdown)
	return nil
}

// cleanupAfterInitFailure unwinds providers that were successfully created
// before a later step failed. Bounded by cleanupTimeout so a stuck exporter
// cannot deadlock application startup. Called via deferred check in Init.
func (t *Telemetry) cleanupAfterInitFailure() {
	if len(t.shutdowns) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	for _, fn := range t.shutdowns {
		_ = fn(ctx)
	}
	t.shutdowns = nil
	t.tp = noopTracerProvider
	t.mp = noopMeterProvider
	t.promHandler = nil
}
